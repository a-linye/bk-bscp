/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package gse provides gse service.
package gse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
)

const (
	// DefaultStartCheckSecs 默认启动后检查存活的时间（秒）
	DefaultStartCheckSecs = 5

	// DefaultOpTimeout 默认命令执行超时时间（秒）
	DefaultOpTimeout = 60

	defaultMaxWait  = 15 * time.Second
	DefaultInterval = 3 * time.Second

	// MaxInProgressRetries 容忍 GSE 任务处于执行中(115)的最大轮询次数，超过即提前返回当前结果。
	MaxInProgressRetries = 150

	// bizSyncBatchSize 按业务全量同步时单个 GSE 任务合并下发的进程实例数量上限。
	bizSyncBatchSize = 1000

	// bizSyncConcurrency 按业务全量同步时并发处理的批次数量上限。
	bizSyncConcurrency = 10

	// bizSyncInterval 按业务全量同步的结果轮询间隔。
	bizSyncInterval = 1500 * time.Millisecond

	// bizSyncMaxWait 按业务全量同步单批次结果的最长等待时间。
	bizSyncMaxWait = 5 * time.Minute
)

// NewSyncGESService 初始化同步gse
func NewSyncGESService(tenantID string, bizID int, svc *gse.Service, dao dao.Set) *syncGSEService {
	return &syncGSEService{
		tenantID: tenantID,
		bizID:    bizID,
		svc:      svc,
		dao:      dao,
	}
}

// syncGSEService 同步gse
type syncGSEService struct {
	tenantID string
	bizID    int
	svc      *gse.Service
	dao      dao.Set
}

// bizOperateItem 表示按业务全量同步时，单个进程实例对应的一次 GSE 查询操作。
// 携带结果 key 与实例指针，便于跨进程合并下发后再把结果映射回对应实例。
type bizOperateItem struct {
	key     string
	operate gse.ProcessOperate
	inst    *table.ProcessInstance
}

// filterSyncableProcesses 仅保留 agent 状态为 normal 的进程。
func filterSyncableProcesses(processes []*table.Process) []*table.Process {
	result := make([]*table.Process, 0, len(processes))
	for _, p := range processes {
		if p.Spec == nil || p.Spec.AgentStatus != table.AgentStatusNormal {
			continue
		}
		result = append(result, p)
	}
	return result
}

// SyncSingleBiz 按业务全量同步 GSE 进程状态。
// 处理流程：
//  1. 一次性取出业务下全部进程实例；
//  2. 将全部实例按 bizSyncBatchSize 切分为多个批次，每批合并为一个 GSE 任务下发；
//  3. 以批次为单位并发处理，统一汇总后批量落库。
//
// 状态判定与字段语义保持不变；单批失败不阻断整体同步。
// nolint:funlen
func (s *syncGSEService) SyncSingleBiz(ctx context.Context) error {
	start := time.Now()
	defer func() {
		logs.Infof("[SyncSingleBiz] biz %d: total cost=%s", s.bizID, time.Since(start))
	}()

	kit := kit.FromGrpcContext(ctx)
	kit.TenantID = s.tenantID
	kit.Ctx = kit.InternalRpcCtx()

	listStart := time.Now()
	processes, err := s.dao.Process().ListProcessesWithInstance(kit, uint32(s.bizID))
	if err != nil {
		logs.Errorf("list active processes failed: %v", err)
		return err
	}
	if len(processes) == 0 {
		logs.Infof("no active processes found, skip sync")
		return nil
	}
	logs.Infof("[SyncSingleBiz] biz %d: list processes done, count=%d, cost=%s",
		s.bizID, len(processes), time.Since(listStart))

	// agent 非 normal 的主机，GSE 查询会长期返回 115 直至超时，导致整批次进程同步失败
	processes = filterSyncableProcesses(processes)
	if len(processes) == 0 {
		logs.Infof("biz %d: no normal-agent processes, skip sync", s.bizID)
		return nil
	}

	// 一次性取出业务下全部进程实例，避免每进程一次 GetByProcessIDs 查询
	processIDs := make([]uint32, 0, len(processes))
	for _, p := range processes {
		processIDs = append(processIDs, p.ID)
	}
	instStart := time.Now()
	insts, err := s.dao.ProcessInstance().GetByProcessIDs(kit, uint32(s.bizID), processIDs)
	if err != nil {
		logs.Errorf("biz %d: get instances failed, err=%v", s.bizID, err)
		return err
	}
	if len(insts) == 0 {
		return nil
	}
	logs.Infof("[SyncSingleBiz] biz %d: get instances done, count=%d, cost=%s",
		s.bizID, len(insts), time.Since(instStart))

	instsByProcess := make(map[uint32][]*table.ProcessInstance, len(processes))
	for _, inst := range insts {
		instsByProcess[inst.Attachment.ProcessID] = append(instsByProcess[inst.Attachment.ProcessID], inst)
	}

	// 构建全业务的 GSE 操作项，并按批合并下发，减少任务数量
	// 该阶段包含模板渲染，是历史上的主要耗时点，单独统计
	buildStart := time.Now()
	batches := chunkBizOperateItems(
		buildBizOperateItems(processes, instsByProcess, uint32(s.bizID)), bizSyncBatchSize)
	logs.Infof("[SyncSingleBiz] biz %d: build operate items done, batches=%d, cost=%s",
		s.bizID, len(batches), time.Since(buildStart))
	if len(batches) == 0 {
		return nil
	}

	gseStart := time.Now()
	var wg sync.WaitGroup
	sem := make(chan struct{}, bizSyncConcurrency)
	resultCh := make(chan []*table.ProcessInstance, len(batches))

	for _, b := range batches {
		batch := b
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if updated := s.syncBizBatch(kit.Ctx, batch); len(updated) > 0 {
				resultCh <- updated
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var allInsts []*table.ProcessInstance
	for updated := range resultCh {
		allInsts = append(allInsts, updated...)
	}
	logs.Infof("[SyncSingleBiz] biz %d: gse batch sync done, updatedInsts=%d, cost=%s",
		s.bizID, len(allInsts), time.Since(gseStart))

	if len(allInsts) == 0 {
		return nil
	}

	// 仅对有实例状态更新的进程记录同步时间，语义与现状一致
	allProcess := collectSyncedProcesses(processes, allInsts, time.Now().UTC())

	dbStart := time.Now()
	tx := s.dao.GenQuery().Begin()
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil {
				logs.Errorf(
					"[SyncSingleBiz ERROR] biz %d: rollback failed, err=%v",
					s.bizID, err,
				)
			}
		}
	}()

	// 1. 批量更新实例
	if len(allInsts) > 0 {
		if err := s.dao.ProcessInstance().BatchUpdateWithTx(kit, tx, allInsts); err != nil {
			return err
		}
	}

	// 2. Process 更新
	if len(allProcess) > 0 {
		if err := s.dao.Process().BatchUpdateWithTx(kit, tx, allProcess); err != nil {
			logs.Errorf("[SyncSingleBiz ERROR] biz %d: update processes failed, err=%v", s.bizID, err)
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		logs.Errorf("[SyncSingleBiz ERROR] biz %d: commit failed, err=%v", s.bizID, err)
		return err
	}
	committed = true
	logs.Infof("[SyncSingleBiz] biz %d: db write done, insts=%d, processes=%d, cost=%s",
		s.bizID, len(allInsts), len(allProcess), time.Since(dbStart))

	return nil
}

// SyncSingleProcessStatus 根据实例同步单个进程的 GSE 状态
// - 成功：返回状态已更新的 insts（可能是部分）
// - 失败：返回 insts + error
func (s *syncGSEService) SyncSingleProcessStatus(ctx context.Context, process *table.Process, insts []*table.ProcessInstance) (
	*table.Process, []*table.ProcessInstance, error) {

	req, instMap, err := buildGSEOperateReq(process, insts, uint32(s.bizID))
	if err != nil {
		logs.Errorf("[SyncSingleBiz ERROR] biz %d: build GSE operate request failed, processID=%d, err=%v",
			s.bizID, process.ID, err)
		return process, insts, err
	}

	proc, err := s.svc.OperateProcMulti(ctx, &gse.MultiProcOperateReq{
		ProcOperateReq: req,
	})
	if err != nil {
		return process, insts, fmt.Errorf(
			"biz %d: operate process failed, processID=%d: %w",
			s.bizID, process.ID, err,
		)
	}

	// 5. 等待 GSE 结果
	result, err := waitForProcResult(
		ctx, s.svc, proc.TaskID, defaultMaxWait, DefaultInterval,
	)
	if err != nil {
		return process, insts, fmt.Errorf(
			"biz %d: wait gse result failed, taskID=%s: %w",
			s.bizID, proc.TaskID, err,
		)
	}

	// 6. 解析实例状态
	updatedInsts := make([]*table.ProcessInstance, 0, len(result))
	for key, val := range result {
		inst := instMap[key]

		status, managed := parseGSEProcResult(key, val)
		logs.Infof("[GSESync][DEBUG] biz=%d processID=%d instID=%d key=%s status=%s managed=%s",
			s.bizID, process.ID, inst.ID, key, status, managed)
		inst.Spec.Status = status
		inst.Spec.ManagedStatus = managed
		inst.Spec.StatusUpdatedAt = time.Now().UTC()
		updatedInsts = append(updatedInsts, inst)
	}

	if len(updatedInsts) == 0 {
		logs.Warnf("[GSESync][WARN] biz=%d processID=%d no instances to update after gse result", s.bizID, process.ID)
		return process, insts, nil
	}
	now := time.Now().UTC()

	process.Spec.ProcessStateSyncedAt = &now

	return process, updatedInsts, nil
}

func buildGSEOperateReq(process *table.Process, insts []*table.ProcessInstance, bizID uint32) (
	[]gse.ProcessOperate, map[string]*table.ProcessInstance, error) {
	req := make([]gse.ProcessOperate, 0, len(insts))
	instMap := make(map[string]*table.ProcessInstance, len(insts))
	var processInfo table.ProcessInfo
	if err := json.Unmarshal([]byte(process.Spec.SourceData), &processInfo); err != nil {
		return nil, nil, fmt.Errorf("unmarshal process source data failed: %w", err)
	}
	for _, inst := range insts {
		key := gse.BuildResultKey(process.Attachment.AgentID, bizID, process.Spec.Alias, inst.Spec.HostInstSeq)
		instMap[key] = inst
		processOperate, err := BuildProcessOperate(BuildProcessOperateParams{
			BizID:         bizID,
			Alias:         process.Spec.Alias,
			FuncName:      process.Spec.FuncName,
			AgentID:       []string{process.Attachment.AgentID},
			HostInstSeq:   inst.Spec.HostInstSeq,
			ModuleInstSeq: inst.Spec.ModuleInstSeq,
			SetName:       process.Spec.SetName,
			ModuleName:    process.Spec.ModuleName,
			GseOpType:     gse.OpTypeQuery,
			ProcessInfo:   processInfo,
		})
		if err != nil {
			logs.Errorf("build process operate failed: %w", err)
			continue
		}
		req = append(req, *processOperate)
	}

	return req, instMap, nil
}

// buildBizOperateItems 把业务下全部进程及其实例构建成扁平的 GSE 操作项列表，
// 用于跨进程合并下发。单个进程的 source_data 解析失败或单个实例构建失败时仅跳过该项，
// 不影响其它进程/实例（沿用 buildGSEOperateReq 的容错策略）。
func buildBizOperateItems(processes []*table.Process,
	instsByProcess map[uint32][]*table.ProcessInstance, bizID uint32) []bizOperateItem {

	items := make([]bizOperateItem, 0)
	skippedNoAgent := 0
	for _, process := range processes {
		insts := instsByProcess[process.ID]
		if len(insts) == 0 {
			continue
		}

		// 跳过 agentID 为空的异常进程记录
		if process.Attachment.AgentID == "" {
			skippedNoAgent += len(insts)
			continue
		}

		var processInfo table.ProcessInfo
		if err := json.Unmarshal([]byte(process.Spec.SourceData), &processInfo); err != nil {
			logs.Errorf("biz %d: unmarshal process source data failed, processID=%d, err=%v",
				bizID, process.ID, err)
			continue
		}

		for _, inst := range insts {
			key := gse.BuildResultKey(process.Attachment.AgentID, bizID, process.Spec.Alias, inst.Spec.HostInstSeq)
			processOperate, err := BuildProcessOperate(BuildProcessOperateParams{
				BizID:         bizID,
				Alias:         process.Spec.Alias,
				FuncName:      process.Spec.FuncName,
				AgentID:       []string{process.Attachment.AgentID},
				HostInstSeq:   inst.Spec.HostInstSeq,
				ModuleInstSeq: inst.Spec.ModuleInstSeq,
				SetName:       process.Spec.SetName,
				ModuleName:    process.Spec.ModuleName,
				GseOpType:     gse.OpTypeQuery,
				ProcessInfo:   processInfo,
			})
			if err != nil {
				logs.Errorf("biz %d: build process operate failed, processID=%d, err=%v",
					bizID, process.ID, err)
				continue
			}
			items = append(items, bizOperateItem{key: key, operate: *processOperate, inst: inst})
		}
	}

	if skippedNoAgent > 0 {
		logs.Warnf("biz %d: skip %d instances of processes with empty agentID (abnormal records, not dispatched to GSE)",
			bizID, skippedNoAgent)
	}

	return items
}

// chunkBizOperateItems 按 size 把操作项切分为多个批次，每批对应一个 GSE 任务。
// size <= 0 时退化为单批，避免出现空批或除零。
func chunkBizOperateItems(items []bizOperateItem, size int) [][]bizOperateItem {
	if len(items) == 0 {
		return nil
	}
	if size <= 0 {
		return [][]bizOperateItem{items}
	}

	batches := make([][]bizOperateItem, 0, (len(items)+size-1)/size)
	for start := 0; start < len(items); start += size {
		end := min(start+size, len(items))
		batches = append(batches, items[start:end])
	}
	return batches
}

// syncBizBatch 处理单个批次：合并下发一个 GSE 任务、轮询结果并解析实例状态。
// 批次内任意环节失败仅记录日志并返回已更新实例（可能为空），不阻断其它批次。
func (s *syncGSEService) syncBizBatch(ctx context.Context, batch []bizOperateItem) []*table.ProcessInstance {
	req, instMap := buildBatchInstMap(batch)

	proc, err := s.svc.OperateProcMulti(ctx, &gse.MultiProcOperateReq{ProcOperateReq: req})
	if err != nil {
		logs.Errorf("biz %d: operate proc failed, batchSize=%d, err=%v", s.bizID, len(batch), err)
		return nil
	}

	result, err := waitForProcResult(ctx, s.svc, proc.TaskID, bizSyncMaxWait, bizSyncInterval)
	if err != nil {
		logs.Errorf("biz %d: wait result failed, taskID=%s, err=%v", s.bizID, proc.TaskID, err)
		return nil
	}

	return applyBatchResult(result, instMap)
}

// buildBatchInstMap 构建下发请求与「结果 key -> 实例列表」映射。
// 同一 GSE result key 可能对应多个实例：HostInstSeq 按进程独立分配，当同 host(agentID)+alias
// 存在多个进程记录（被标记为 abnormal 的冲突进程）时，它们的实例会算出相同的 BuildResultKey。
// 因此这里用多值 map 保存，避免单值映射在合批后静默覆盖、丢失前序实例的更新。
func buildBatchInstMap(batch []bizOperateItem) ([]gse.ProcessOperate, map[string][]*table.ProcessInstance) {
	req := make([]gse.ProcessOperate, 0, len(batch))
	instMap := make(map[string][]*table.ProcessInstance, len(batch))
	for _, it := range batch {
		req = append(req, it.operate)
		instMap[it.key] = append(instMap[it.key], it.inst)
	}
	return req, instMap
}

// applyBatchResult 将 GSE 结果按 key 扇出到所有命中实例。
// 与 gsekit 一致：同一 key 命中的全部实例都更新为该 key 对应的同一状态，不丢更新。
func applyBatchResult(result map[string]gse.ProcResult,
	instMap map[string][]*table.ProcessInstance) []*table.ProcessInstance {

	updatedInsts := make([]*table.ProcessInstance, 0, len(result))
	for key, val := range result {
		insts := instMap[key]
		if len(insts) == 0 {
			continue
		}
		// GSE 仍在执行中（115）说明本轮拿不到确定状态，跳过更新以保留实例原状态，
		// 避免把"执行中"误判为 stopped/unmanaged 覆盖正确数据，等待下次同步重新拉取。
		if gse.IsInProgress(val.ErrorCode) {
			continue
		}
		status, managed := parseGSEProcResult(key, val)
		for _, inst := range insts {
			inst.Spec.Status = status
			inst.Spec.ManagedStatus = managed
			updatedInsts = append(updatedInsts, inst)
		}
	}

	return updatedInsts
}

// collectSyncedProcesses 根据已更新实例反查需要标记同步时间的进程：
// 仅当某进程至少有一个实例状态被更新时，才写入 ProcessStateSyncedAt 并纳入更新集合，
// 与逐进程同步的语义保持一致。
func collectSyncedProcesses(processes []*table.Process,
	updatedInsts []*table.ProcessInstance, syncedAt time.Time) []*table.Process {

	touched := make(map[uint32]struct{}, len(updatedInsts))
	for _, inst := range updatedInsts {
		touched[inst.Attachment.ProcessID] = struct{}{}
	}

	result := make([]*table.Process, 0, len(touched))
	for _, process := range processes {
		if _, ok := touched[process.ID]; !ok {
			continue
		}
		ts := syncedAt
		process.Spec.ProcessStateSyncedAt = &ts
		result = append(result, process)
	}
	return result
}

// parseGSEProcResult 解析 GSE 返回结果
func parseGSEProcResult(key string, v gse.ProcResult) (status table.ProcessStatus, managed table.ProcessManagedStatus) {
	status = table.ProcessStatusStopped
	managed = table.ProcessManagedStatusUnmanaged

	switch v.ErrorCode {
	case gse.ErrCodeSuccess:
		var contents gse.ProcessStatusContent
		if err := json.Unmarshal([]byte(v.Content), &contents); err != nil {
			logs.Warnf("unmarshal success content failed for %s: %v", key, err)
			return status, managed
		}
		status = table.ProcessStatusStarting
		for _, p := range contents.Process {
			for _, i := range p.Instance {
				if i.IsAuto {
					managed = table.ProcessManagedStatusManaged
				}
				if i.PID > 0 {
					status = table.ProcessStatusRunning
				}
				// 启动失败了
				if i.PID < 0 {
					status = table.ProcessStatusStopped
				}
			}
		}

	case gse.ErrCodeStopping:
		var contents gse.StoppingContent
		if err := json.Unmarshal([]byte(v.Content), &contents); err != nil {
			logs.Warnf("unmarshal stopping content failed for %s: %v", key, err)
			return status, managed
		}
		status = table.ProcessStatusStopping
		for _, c := range contents.Value {
			if c.IsAuto {
				managed = table.ProcessManagedStatusManaged
			}
		}

	default:
		logs.Warnf("key=%s, unknown gse code: %d", key, v.ErrorCode)
	}

	return status, managed
}

func waitForProcResult(ctx context.Context, svc *gse.Service, taskID string,
	maxWait, interval time.Duration) (map[string]gse.ProcResult, error) {

	var result map[string]gse.ProcResult
	inProgressCount := 0

	err := wait.PollUntilContextTimeout(
		ctx,
		interval,
		maxWait,
		true,
		func(ctx context.Context) (bool, error) {
			resp, err := svc.GetProcOperateResultV2(ctx, &gse.QueryProcResultReq{TaskID: taskID})
			if err != nil {
				return false, fmt.Errorf("get process result failed, taskID=%s, err=%v", taskID, err)
			}

			if resp.Code != 0 {
				return false, fmt.Errorf("gse API error, code=%d, msg=%s", resp.Code, resp.Message)
			}

			// 解码结果
			if err := resp.Decode(&result); err != nil {
				return false, fmt.Errorf("decode gse result failed, taskID=%s, err=%v", taskID, err)
			}

			// 是否存在正在执行的进程即 115 状态码
			hasInProgress := false
			// 所有正在执行的进程即 115 状态码
			var inProgressProcs []gse.ProcResult

			// 检查是否还有实例在进行中
			for _, proc := range result {
				if gse.IsInProgress(proc.ErrorCode) { // 进程仍在执行中
					hasInProgress = true
					inProgressProcs = append(inProgressProcs, proc)
				}
			}

			if hasInProgress {
				inProgressCount++
				if inProgressCount > MaxInProgressRetries {
					// 详细日志：包含任务ID、计数，以及只包含 115 的进程条目
					logs.Warnf("task=%s: seen ErrorCode==115 for %d times — still in progress; "+
						"return current result (in-progress entries skipped by caller); count of 115 procs=%d",
						taskID, inProgressCount, len(inProgressProcs))
					// 超过重试上限依旧在执行 → 提前返回当前结果：
					// 已完成的进程正常落库，残留 115 由 applyBatchResult 跳过、保留原状态。
					return true, nil
				}
				return false, nil // 继续轮询
			}

			return true, nil // 全部完成
		},
	)

	if err != nil {
		if wait.Interrupted(err) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("timeout or canceled waiting for GSE result, taskID=%s", taskID)
		}
		return nil, err
	}

	return result, nil
}
