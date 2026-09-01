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
	"reflect"
	"testing"
)

func TestProcessOperatePriorityOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		op        ProcessOperateType
		wantOrder PriorityOrder
		wantOK    bool
	}{
		{"start 升序", StartProcessOperate, PriorityOrderAsc, true},
		{"reload 升序", ReloadProcessOperate, PriorityOrderAsc, true},
		{"kill 升序", KillProcessOperate, PriorityOrderAsc, true},
		{"stop 降序", StopProcessOperate, PriorityOrderDesc, true},
		{"restart 降序", RestartProcessOperate, PriorityOrderDesc, true},
		{"register 不分批", RegisterProcessOperate, "", false},
		{"unregister 不分批", UnregisterProcessOperate, "", false},
		{"update_register 不分批", UpdateRegisterProcessOperate, "", false},
		{"delete 不直接进入编排", DeleteProcessOperate, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ProcessOperatePriorityOrder(tt.op)
			if ok != tt.wantOK || got != tt.wantOrder {
				t.Fatalf("ProcessOperatePriorityOrder(%s) = (%q, %v), want (%q, %v)",
					tt.op, got, ok, tt.wantOrder, tt.wantOK)
			}
		})
	}
}

func TestBuildPriorityPlan_StartAsc(t *testing.T) {
	t.Parallel()

	plan, immediate := BuildPriorityPlan([]PriorityTaskItem{
		{TaskID: "b", Priority: 2, OpType: StartProcessOperate},
		{TaskID: "a1", Priority: 1, OpType: StartProcessOperate},
		{TaskID: "a2", Priority: 1, OpType: StartProcessOperate},
		{TaskID: "neg", Priority: -1, OpType: StartProcessOperate},
	})
	if len(immediate) != 0 {
		t.Fatalf("start 不应产生立即下发任务, got %v", immediate)
	}
	if plan == nil || plan.Order != PriorityOrderAsc {
		t.Fatalf("期望升序计划, got %+v", plan)
	}
	if len(plan.Waves) != 3 {
		t.Fatalf("期望 3 个波次, got %d", len(plan.Waves))
	}
	if plan.Waves[0].Priority != -1 || plan.Waves[0].Seq != 1 ||
		!reflect.DeepEqual(plan.Waves[0].TaskIDs, []string{"neg"}) {
		t.Fatalf("第 1 波次应为 priority=-1, got %+v", plan.Waves[0])
	}
	if plan.Waves[1].Priority != 1 || !reflect.DeepEqual(plan.Waves[1].TaskIDs, []string{"a1", "a2"}) {
		t.Fatalf("第 2 波次应为 priority=1 的两个任务, got %+v", plan.Waves[1])
	}
	if plan.Waves[2].Priority != 2 || !reflect.DeepEqual(plan.Waves[2].TaskIDs, []string{"b"}) {
		t.Fatalf("第 3 波次应为 priority=2, got %+v", plan.Waves[2])
	}
}

func TestBuildPriorityPlan_StopDesc(t *testing.T) {
	t.Parallel()

	plan, immediate := BuildPriorityPlan([]PriorityTaskItem{
		{TaskID: "low", Priority: 1, OpType: StopProcessOperate},
		{TaskID: "high", Priority: 3, OpType: StopProcessOperate},
	})
	if len(immediate) != 0 {
		t.Fatalf("stop 不应产生立即下发任务, got %v", immediate)
	}
	if plan == nil || plan.Order != PriorityOrderDesc {
		t.Fatalf("期望降序计划, got %+v", plan)
	}
	if len(plan.Waves) != 2 {
		t.Fatalf("期望 2 个波次, got %d", len(plan.Waves))
	}
	if plan.Waves[0].Priority != 3 || plan.Waves[1].Priority != 1 {
		t.Fatalf("stop 应按 priority 降序, got %d then %d", plan.Waves[0].Priority, plan.Waves[1].Priority)
	}
}

func TestBuildPriorityPlan_ManagedImmediate(t *testing.T) {
	t.Parallel()

	plan, immediate := BuildPriorityPlan([]PriorityTaskItem{
		{TaskID: "r1", Priority: 1, OpType: RegisterProcessOperate},
		{TaskID: "r2", Priority: 9, OpType: UnregisterProcessOperate},
		{TaskID: "r3", Priority: 2, OpType: UpdateRegisterProcessOperate},
	})
	if plan != nil {
		t.Fatalf("托管类不应生成优先级计划, got %+v", plan)
	}
	if !reflect.DeepEqual(immediate, []string{"r1", "r2", "r3"}) {
		t.Fatalf("托管类应全部立即下发, got %v", immediate)
	}
}

func TestBuildPriorityPlan_DeleteSplit(t *testing.T) {
	t.Parallel()

	plan, immediate := BuildPriorityPlan([]PriorityTaskItem{
		{TaskID: "stop-2", Priority: 2, OpType: StopProcessOperate},
		{TaskID: "stop-1", Priority: 1, OpType: StopProcessOperate},
		{TaskID: "unreg-1", Priority: 1, OpType: UnregisterProcessOperate},
		{TaskID: "unreg-2", Priority: 2, OpType: UnregisterProcessOperate},
	})
	if !reflect.DeepEqual(immediate, []string{"unreg-1", "unreg-2"}) {
		t.Fatalf("unregister 分支应立即下发, got %v", immediate)
	}
	if plan == nil || plan.Order != PriorityOrderDesc || len(plan.Waves) != 2 {
		t.Fatalf("stop 分支应按降序分 2 个波次, got %+v", plan)
	}
	if plan.Waves[0].Priority != 2 || plan.Waves[1].Priority != 1 {
		t.Fatalf("stop 分支降序错误: %+v", plan.Waves)
	}
}

func TestBuildPriorityPlan_SamePrioritySingleWave(t *testing.T) {
	t.Parallel()

	plan, immediate := BuildPriorityPlan([]PriorityTaskItem{
		{TaskID: "a", Priority: 0, OpType: StartProcessOperate},
		{TaskID: "b", Priority: 0, OpType: StartProcessOperate},
	})
	if len(immediate) != 0 {
		t.Fatalf("相同 priority 不应产生立即任务, got %v", immediate)
	}
	if plan == nil || len(plan.Waves) != 1 || plan.Waves[0].Priority != 0 {
		t.Fatalf("相同 priority 应只有 1 个波次, got %+v", plan)
	}
	if !reflect.DeepEqual(plan.Waves[0].TaskIDs, []string{"a", "b"}) {
		t.Fatalf("单波次应包含全部任务, got %v", plan.Waves[0].TaskIDs)
	}
}

func TestAdvancePriorityPlan_DispatchNextOnAllSuccess(t *testing.T) {
	t.Parallel()

	plan := &PriorityPlan{
		Order: PriorityOrderAsc,
		Waves: []*PriorityWave{
			{Seq: 1, Priority: 1, TaskIDs: []string{"a", "b"}},
			{Seq: 2, Priority: 2, TaskIDs: []string{"c"}},
		},
	}

	next, cascade := AdvancePriorityPlan(plan, 1, true)
	if next != nil || cascade != nil {
		t.Fatalf("首个任务成功不应推进, next=%+v cascade=%+v", next, cascade)
	}
	if plan.Waves[0].Completed != 1 || plan.Waves[0].Failed != 0 {
		t.Fatalf("波次计数错误: %+v", plan.Waves[0])
	}

	next, cascade = AdvancePriorityPlan(plan, 1, true)
	if cascade != nil {
		t.Fatalf("全部成功不应级联, got %+v", cascade)
	}
	if next == nil || next.Seq != 2 || next.Priority != 2 {
		t.Fatalf("全部成功应下发下一波, got %+v", next)
	}
	if plan.CurrentWave != 1 {
		t.Fatalf("CurrentWave 应为 1, got %d", plan.CurrentWave)
	}
}

func TestAdvancePriorityPlan_CascadeOnPartialFail(t *testing.T) {
	t.Parallel()

	plan := &PriorityPlan{
		Order: PriorityOrderAsc,
		Waves: []*PriorityWave{
			{Seq: 1, Priority: 1, TaskIDs: []string{"a", "b"}},
			{Seq: 2, Priority: 2, TaskIDs: []string{"c", "d"}},
			{Seq: 3, Priority: 3, TaskIDs: []string{"e"}},
		},
	}

	AdvancePriorityPlan(plan, 1, false)
	next, cascade := AdvancePriorityPlan(plan, 1, true)
	if next != nil {
		t.Fatalf("存在失败时不应下发下一波, got %+v", next)
	}
	if cascade == nil || cascade.FailedPriority != 1 || cascade.Order != PriorityOrderAsc {
		t.Fatalf("级联信息不对, got %+v", cascade)
	}
	if !reflect.DeepEqual(cascade.PendingTaskIDs, []string{"c", "d", "e"}) {
		t.Fatalf("应阻断后续全部任务, got %v", cascade.PendingTaskIDs)
	}
	if !plan.Blocked || plan.BlockedPriority != 1 {
		t.Fatalf("计划应标记为已阻断, got blocked=%v priority=%d", plan.Blocked, plan.BlockedPriority)
	}
	if !plan.Waves[1].Dispatched || plan.Waves[1].Failed != 2 || plan.Waves[2].Failed != 1 {
		t.Fatalf("后续波次应标记为已完成失败, wave2=%+v wave3=%+v", plan.Waves[1], plan.Waves[2])
	}
}

func TestAdvancePriorityPlan_LastWaveFailNoCascade(t *testing.T) {
	t.Parallel()

	plan := &PriorityPlan{
		Order: PriorityOrderDesc,
		Waves: []*PriorityWave{
			{Seq: 1, Priority: 3, TaskIDs: []string{"a"}},
		},
	}
	next, cascade := AdvancePriorityPlan(plan, 1, false)
	if next != nil || cascade != nil {
		t.Fatalf("最后一波失败不应级联, next=%+v cascade=%+v", next, cascade)
	}
	if !plan.Blocked {
		t.Fatalf("最后一波失败仍应标记阻断")
	}
}

func TestAdvancePriorityPlan_ImmediateAndBlockedSkip(t *testing.T) {
	t.Parallel()

	plan := &PriorityPlan{
		Order:   PriorityOrderAsc,
		Blocked: true,
		Waves:   []*PriorityWave{{Seq: 1, Priority: 1, TaskIDs: []string{"a"}}},
	}
	next, cascade := AdvancePriorityPlan(plan, 1, true)
	if next != nil || cascade != nil {
		t.Fatalf("已阻断计划不应再推进")
	}

	next, cascade = AdvancePriorityPlan(plan, ImmediateWaveSeq, false)
	if next != nil || cascade != nil {
		t.Fatalf("立即波次不应参与编排")
	}
	if plan.Waves[0].Completed != 0 {
		t.Fatalf("立即波次/已阻断不应改写波次计数")
	}
}

func TestFirstDispatchTaskIDs(t *testing.T) {
	t.Parallel()

	plan, immediate := BuildPriorityPlan([]PriorityTaskItem{
		{TaskID: "a", Priority: 1, OpType: StartProcessOperate},
		{TaskID: "b", Priority: 2, OpType: StartProcessOperate},
		{TaskID: "u", Priority: 9, OpType: UnregisterProcessOperate},
	})
	got := FirstDispatchTaskIDs(plan, immediate)
	if !reflect.DeepEqual(got, []string{"u", "a"}) {
		t.Fatalf("应立即下发 unregister 与第一波, got %v", got)
	}
	if plan.CurrentWave != 0 {
		t.Fatalf("CurrentWave 应为 0, got %d", plan.CurrentWave)
	}
	if plan.Waves[0].Dispatched {
		t.Fatalf("首波下发标记应在实际入队后写入")
	}
}

func TestCascadeBlockMessage(t *testing.T) {
	t.Parallel()

	asc := CascadeBlockMessage(1, PriorityOrderAsc)
	if asc != "优先级等于[1]的进程操作已失败，优先级大于此的进程操作不会被继续执行" {
		t.Fatalf("升序文案不符: %s", asc)
	}
	desc := CascadeBlockMessage(3, PriorityOrderDesc)
	if desc != "优先级等于[3]的进程操作已失败，优先级小于此的进程操作不会被继续执行" {
		t.Fatalf("降序文案不符: %s", desc)
	}
}

func TestPriorityPlan_WaveSeqOf(t *testing.T) {
	t.Parallel()

	plan := &PriorityPlan{
		Waves: []*PriorityWave{
			{Seq: 1, TaskIDs: []string{"a"}},
			{Seq: 2, TaskIDs: []string{"b", "c"}},
		},
	}
	if got := plan.WaveSeqOf("c"); got != 2 {
		t.Fatalf("WaveSeqOf(c) = %d, want 2", got)
	}
	if got := plan.WaveSeqOf("missing"); got != ImmediateWaveSeq {
		t.Fatalf("未知任务应回落 ImmediateWaveSeq, got %d", got)
	}
}

func TestTaskBatchExtraData_PreservesRegisterProcess(t *testing.T) {
	t.Parallel()

	spec := &TaskBatchSpec{}
	if err := spec.SetExtraData(&TaskBatchExtraData{
		PriorityPlan: &PriorityPlan{Order: PriorityOrderAsc},
		RegisterProcess: &RegisterProcessExtra{
			SuccessCount: 3,
		},
	}); err != nil {
		t.Fatalf("SetExtraData: %v", err)
	}
	got, err := spec.GetExtraData()
	if err != nil {
		t.Fatalf("GetExtraData: %v", err)
	}
	if got.PriorityPlan == nil || got.PriorityPlan.Order != PriorityOrderAsc {
		t.Fatalf("PriorityPlan 丢失: %+v", got.PriorityPlan)
	}
	if got.RegisterProcess == nil || got.RegisterProcess.SuccessCount != 3 {
		t.Fatalf("RegisterProcess 丢失: %+v", got.RegisterProcess)
	}
}
