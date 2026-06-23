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
	// Maximum number of times to tolerate ErrorCode 115 before treating as complete
	MaxInProgressRetries = 5

	// bizSyncBatchSize 按业务全量同步时单个 GSE 任务合并下发的进程实例数量上限。
	bizSyncBatchSize = 1000

	// bizSyncConcurrency 按业务全量同步时并发处理的批次数量上限。
	bizSyncConcurrency = 10

	// bizSyncInterval 按业务全量同步的结果轮询间隔。
	bizSyncInterval = 1500 * time.Millisecond

	// bizSyncMaxWait 按业务全量同步单批次结果的最长等待时间。
	bizSyncMaxWait = 30 * time.Second
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

// SyncSingleBiz 按业务全量同步 GSE 进程状态。
// 处理流程：
//  1. 一次性取出业务下全部进程实例；
//  2. 将全部实例按 bizSyncBatchSize 切分为多个批次，每批合并为一个 GSE 任务下发；
//  3. 以批次为单位并发处理，统一汇总后批量落库。
//
// 状态判定与字段语义保持不变；单批失败不阻断整体同步。
func (s *syncGSEService) SyncSingleBiz(ctx context.Context) error {
	kit := kit.FromGrpcContext(ctx)
	kit.TenantID = s.tenantID
	kit.Ctx = kit.InternalRpcCtx()
	processes, err := s.dao.Process().ListProcessesWithInstance(kit, uint32(s.bizID))
	if err != nil {
		logs.Errorf("list active processes failed: %v", err)
		return err
	}
	if len(processes) == 0 {
		logs.Infof("no active processes found, skip sync")
		return nil
	}

	// 一次性取出业务下全部进程实例，避免每进程一次 GetByProcessIDs 查询
	processIDs := make([]uint32, 0, len(processes))
	for _, p := range processes {
		processIDs = append(processIDs, p.ID)
	}
	insts, err := s.dao.ProcessInstance().GetByProcessIDs(kit, uint32(s.bizID), processIDs)
	if err != nil {
		logs.Errorf("biz %d: get instances failed, err=%v", s.bizID, err)
		return err
	}
	if len(insts) == 0 {
		return nil
	}

	instsByProcess := make(map[uint32][]*table.ProcessInstance, len(processes))
	for _, inst := range insts {
		instsByProcess[inst.Attachment.ProcessID] = append(instsByProcess[inst.Attachment.ProcessID], inst)
	}

	// 构建全业务的 GSE 操作项，并按批合并下发，减少任务数量
	batches := chunkBizOperateItems(
		buildBizOperateItems(processes, instsByProcess, uint32(s.bizID)), bizSyncBatchSize)
	if len(batches) == 0 {
		return nil
	}

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

	if len(allInsts) == 0 {
		return nil
	}

	// 仅对有实例状态更新的进程记录同步时间，语义与现状一致
	allProcess := collectSyncedProcesses(processes, allInsts, time.Now().UTC())

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
	for _, process := range processes {
		insts := instsByProcess[process.ID]
		if len(insts) == 0 {
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
	req := make([]gse.ProcessOperate, 0, len(batch))
	instMap := make(map[string]*table.ProcessInstance, len(batch))
	for _, it := range batch {
		req = append(req, it.operate)
		instMap[it.key] = it.inst
	}

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

	updatedInsts := make([]*table.ProcessInstance, 0, len(result))
	for key, val := range result {
		inst := instMap[key]
		if inst == nil {
			continue
		}
		status, managed := parseGSEProcResult(key, val)
		inst.Spec.Status = status
		inst.Spec.ManagedStatus = managed
		updatedInsts = append(updatedInsts, inst)
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

func waitForProcResult(ctx context.Context, svc *gse.Service, taskID string, maxWait, interval time.Duration) (map[string]gse.ProcResult, error) {
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
					// 详细日志：包含业务ID、任务ID、计数，以及只包含115的进程条目
					logs.Warnf("task=%s: seen ErrorCode==115 for %d times — still in progress; listing procs with 115: %+v",
						taskID, inProgressCount, inProgressProcs)
					// 超过5次依旧是正在执行 → 直接认为完成
					return true, nil
				}
				return false, nil // 继续轮询
			}

			return true, nil // 全部完成
		},
	)

	if err != nil {
		if wait.Interrupted(err) || err == context.DeadlineExceeded || err == context.Canceled {
			return nil, fmt.Errorf("timeout or canceled waiting for GSE result, taskID=%s", taskID)
		}
		return nil, err
	}

	return result, nil
}
