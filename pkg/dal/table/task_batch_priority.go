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

package table

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// PriorityOrder 优先级波次的排序方向
type PriorityOrder string

const (
	// PriorityOrderAsc 升序：优先级小的先执行（start / reload / kill）
	PriorityOrderAsc PriorityOrder = "asc"
	// PriorityOrderDesc 降序：优先级大的先执行（stop / restart）
	PriorityOrderDesc PriorityOrder = "desc"
)

// ImmediateWaveSeq 不参与优先级编排的任务所使用的波次序号。
// 托管类操作（register / unregister / update_register）以及 delete 拆解出的 unregister 分支
// 全部归入该序号，创建批次时一次性下发，不做跨优先级串行等待。
const ImmediateWaveSeq = 0

// PriorityWave 一个优先级波次：同一 priority 的实例任务归为一批，批内并行
type PriorityWave struct {
	// Seq 波次序号，自 1 递增，与执行顺序一致
	Seq int `json:"seq"`
	// Priority 该波次的进程启动优先级
	Priority int `json:"priority"`
	// TaskIDs 该波次内全部实例任务的 ID，批次创建时即全量落库
	TaskIDs []string `json:"task_ids"`
	// Dispatched 该波次是否已下发
	Dispatched bool `json:"dispatched"`
	// Completed 该波次已到达终态的任务数
	Completed int `json:"completed"`
	// Failed 该波次非成功终态（失败或超时）的任务数
	Failed int `json:"failed"`
}

// Total 波次内的实例任务总数
func (w *PriorityWave) Total() int {
	return len(w.TaskIDs)
}

// PriorityPlan 一次进程操作请求的优先级编排计划。
// 计划在批次创建时生成并持久化，服务重启后可据此恢复推进。
type PriorityPlan struct {
	// Order 排序方向，由操作类型决定
	Order PriorityOrder `json:"order"`
	// Waves 按 Order 排好序的波次序列
	Waves []*PriorityWave `json:"waves"`
	// CurrentWave 当前正在执行的波次下标（Waves 的下标，从 0 开始）
	CurrentWave int `json:"current_wave"`
	// Blocked 是否已因某个波次未全部成功而阻断后续波次
	Blocked bool `json:"blocked"`
	// BlockedPriority 触发阻断的波次优先级值，用于生成失败原因文案
	BlockedPriority int `json:"blocked_priority,omitempty"`
}

// WaveBySeq 按波次序号查找波次
func (p *PriorityPlan) WaveBySeq(seq int) *PriorityWave {
	for _, w := range p.Waves {
		if w.Seq == seq {
			return w
		}
	}
	return nil
}

// PendingTaskIDsAfter 返回指定波次之后所有尚未下发波次的任务 ID
func (p *PriorityPlan) PendingTaskIDsAfter(waveIndex int) []string {
	var taskIDs []string
	for i := waveIndex + 1; i < len(p.Waves); i++ {
		taskIDs = append(taskIDs, p.Waves[i].TaskIDs...)
	}
	return taskIDs
}

// WaveIndex 返回波次序号对应的下标，未找到返回 -1
func (p *PriorityPlan) WaveIndex(seq int) int {
	for i, w := range p.Waves {
		if w.Seq == seq {
			return i
		}
	}
	return -1
}

// WaveSeqOf 返回任务所属波次序号；未纳入编排时回落为 ImmediateWaveSeq
func (p *PriorityPlan) WaveSeqOf(taskID string) int {
	if p == nil {
		return ImmediateWaveSeq
	}
	for _, w := range p.Waves {
		for _, id := range w.TaskIDs {
			if id == taskID {
				return w.Seq
			}
		}
	}
	return ImmediateWaveSeq
}

// PriorityTaskItem 构建优先级计划时的单个实例任务
type PriorityTaskItem struct {
	TaskID   string
	Priority int
	OpType   ProcessOperateType
}

// BuildPriorityPlan 按操作类型对实例任务分组：
// 1. 不参与优先级编排的操作（托管类 / delete 拆出的 unregister 等）归入立即下发列表
// 2. 参与编排的操作按 priority 分组，再按操作方向（升序 / 降序）排成波次
// 3. 没有任何任务需要分波次时返回 nil 计划，调用方只下发 immediate
func BuildPriorityPlan(items []PriorityTaskItem) (*PriorityPlan, []string) {
	immediate := make([]string, 0)
	groups := make(map[int][]string)
	var order PriorityOrder
	hasOrdered := false

	// 1. 按操作类型分流：立即下发 vs 按 priority 分组
	for _, item := range items {
		o, ok := ProcessOperatePriorityOrder(item.OpType)
		if !ok {
			immediate = append(immediate, item.TaskID)
			continue
		}
		// 同一次请求内的排序方向，由首个参与编排的任务决定
		if !hasOrdered {
			order = o
			hasOrdered = true
		}
		groups[item.Priority] = append(groups[item.Priority], item.TaskID)
	}

	// 2. 全部都是立即任务时，无需编排计划
	if !hasOrdered {
		return nil, immediate
	}

	// 3. 收集 priority，默认升序；stop / restart 再反转为降序
	prios := make([]int, 0, len(groups))
	for p := range groups {
		prios = append(prios, p)
	}
	sort.Ints(prios)
	if order == PriorityOrderDesc {
		for i, j := 0, len(prios)-1; i < j; i, j = i+1, j-1 {
			prios[i], prios[j] = prios[j], prios[i]
		}
	}

	// 4. 每个 priority 生成一个波次，Seq 从 1 递增，与执行顺序一致
	plan := &PriorityPlan{
		Order: order,
		Waves: make([]*PriorityWave, 0, len(prios)),
	}
	for i, p := range prios {
		plan.Waves = append(plan.Waves, &PriorityWave{
			Seq:      i + 1,
			Priority: p,
			TaskIDs:  groups[p],
		})
	}
	return plan, immediate
}

// AdvancePriorityPlan 在同一波次内累加终态计数，并在波次结束时决定：
// 全部成功则返回下一波次；存在失败则阻断后续波次并返回级联信息。
// waveSeq 为 ImmediateWaveSeq、计划为空或已阻断时不做编排推进。
func AdvancePriorityPlan(plan *PriorityPlan, waveSeq int, isSuccess bool) (*PriorityWave, *PriorityCascade) {
	if plan == nil || waveSeq == ImmediateWaveSeq || plan.Blocked {
		return nil, nil
	}
	idx := plan.WaveIndex(waveSeq)
	if idx < 0 {
		return nil, nil
	}
	wave := plan.Waves[idx]
	wave.Completed++
	if !isSuccess {
		wave.Failed++
	}
	if wave.Completed < wave.Total() {
		return nil, nil
	}
	if wave.Failed > 0 {
		plan.Blocked = true
		plan.BlockedPriority = wave.Priority
		plan.markPendingFailed(idx)
		pending := plan.PendingTaskIDsAfter(idx)
		if len(pending) == 0 {
			return nil, nil
		}
		return nil, &PriorityCascade{
			FailedPriority: wave.Priority,
			Order:          plan.Order,
			PendingTaskIDs: pending,
		}
	}
	if idx+1 < len(plan.Waves) {
		plan.CurrentWave = idx + 1
		return plan.Waves[idx+1], nil
	}
	return nil, nil
}

func (p *PriorityPlan) markPendingFailed(fromIndex int) {
	for i := fromIndex + 1; i < len(p.Waves); i++ {
		w := p.Waves[i]
		w.Dispatched = true
		w.Completed = w.Total()
		w.Failed = w.Total()
	}
}

// CascadeBlockMessage 生成对齐 gsekit 的优先级级联阻断文案
func CascadeBlockMessage(failedPriority int, order PriorityOrder) string {
	if order == PriorityOrderDesc {
		return fmt.Sprintf("优先级等于[%d]的进程操作已失败，优先级小于此的进程操作不会被继续执行", failedPriority)
	}
	return fmt.Sprintf("优先级等于[%d]的进程操作已失败，优先级大于此的进程操作不会被继续执行", failedPriority)
}

// FirstDispatchTaskIDs 返回批次创建后应立即下发的任务 ID：立即波次 + 第一个优先级波次
func FirstDispatchTaskIDs(plan *PriorityPlan, immediate []string) []string {
	ids := append([]string{}, immediate...)
	if plan != nil && len(plan.Waves) > 0 {
		ids = append(ids, plan.Waves[0].TaskIDs...)
		plan.CurrentWave = 0
	}
	return ids
}

// PriorityCascade 优先级失败级联阻断的信息
type PriorityCascade struct {
	FailedPriority int
	Order          PriorityOrder
	PendingTaskIDs []string
}

// TaskBatchExtraData 任务批次的扩展数据，序列化后存放于 task_batches.extra_data
type TaskBatchExtraData struct {
	// PriorityPlan 进程操作的优先级编排计划，非进程操作或无需编排时为空
	PriorityPlan *PriorityPlan `json:"priority_plan,omitempty"`
	// RegisterProcess 更新托管任务的成功计数，与优先级编排共用 extra_data
	RegisterProcess *RegisterProcessExtra `json:"register_process,omitempty"`
}

// RegisterProcessExtra 更新托管扩展参数
type RegisterProcessExtra struct {
	SuccessCount uint32 `json:"success_count"`
}

// GetExtraData 解析批次扩展数据，extra_data 为空时返回空结构
func (t *TaskBatchSpec) GetExtraData() (*TaskBatchExtraData, error) {
	extra := &TaskBatchExtraData{}
	if t.ExtraData == "" || t.ExtraData == "{}" {
		return extra, nil
	}
	if err := json.Unmarshal([]byte(t.ExtraData), extra); err != nil {
		return nil, err
	}
	return extra, nil
}

// SetExtraData 序列化批次扩展数据
func (t *TaskBatchSpec) SetExtraData(extra *TaskBatchExtraData) error {
	if extra == nil {
		return errors.New("extra data not set")
	}
	b, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	t.ExtraData = string(b)
	return nil
}

// GetPriorityPlan 获取批次的优先级编排计划，未编排时返回 nil
func (t *TaskBatchSpec) GetPriorityPlan() (*PriorityPlan, error) {
	extra, err := t.GetExtraData()
	if err != nil {
		return nil, err
	}
	return extra.PriorityPlan, nil
}
