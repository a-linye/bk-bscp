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

package gse

import (
	"testing"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

func TestChunkBizOperateItems(t *testing.T) {
	makeItems := func(n int) []bizOperateItem {
		items := make([]bizOperateItem, 0, n)
		for i := 0; i < n; i++ {
			items = append(items, bizOperateItem{inst: &table.ProcessInstance{ID: uint32(i + 1)}})
		}
		return items
	}

	tests := []struct {
		name      string
		count     int
		size      int
		wantSizes []int
	}{
		{name: "empty", count: 0, size: 1000, wantSizes: nil},
		{name: "single batch under size", count: 3, size: 1000, wantSizes: []int{3}},
		{name: "exact multiple", count: 2000, size: 1000, wantSizes: []int{1000, 1000}},
		{name: "with remainder", count: 2300, size: 1000, wantSizes: []int{1000, 1000, 300}},
		{name: "size larger than count", count: 5, size: 10, wantSizes: []int{5}},
		{name: "non-positive size degrades to single batch", count: 7, size: 0, wantSizes: []int{7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batches := chunkBizOperateItems(makeItems(tt.count), tt.size)

			if len(batches) != len(tt.wantSizes) {
				t.Fatalf("batches count = %d, want %d", len(batches), len(tt.wantSizes))
			}

			total := 0
			for i, b := range batches {
				if len(b) != tt.wantSizes[i] {
					t.Errorf("batch[%d] size = %d, want %d", i, len(b), tt.wantSizes[i])
				}
				total += len(b)
			}
			if total != tt.count {
				t.Errorf("total items across batches = %d, want %d", total, tt.count)
			}
		})
	}
}

// TestChunkBizOperateItemsNoOverlap 保证切分后所有实例不重复、不遗漏。
func TestChunkBizOperateItemsNoOverlap(t *testing.T) {
	const count = 2500
	items := make([]bizOperateItem, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, bizOperateItem{inst: &table.ProcessInstance{ID: uint32(i + 1)}})
	}

	seen := make(map[uint32]int, count)
	for _, b := range chunkBizOperateItems(items, 1000) {
		for _, it := range b {
			seen[it.inst.ID]++
		}
	}

	if len(seen) != count {
		t.Fatalf("distinct instances = %d, want %d", len(seen), count)
	}
	for id, c := range seen {
		if c != 1 {
			t.Errorf("instance %d appeared %d times, want 1", id, c)
		}
	}
}

func newInst(id, processID uint32) *table.ProcessInstance {
	return &table.ProcessInstance{
		ID:         id,
		Attachment: &table.ProcessInstanceAttachment{ProcessID: processID},
		Spec:       &table.ProcessInstanceSpec{},
	}
}

func newProcess(id uint32) *table.Process {
	return &table.Process{ID: id, Spec: &table.ProcessSpec{}}
}

func TestCollectSyncedProcesses(t *testing.T) {
	syncedAt := time.Date(2026, 6, 23, 8, 0, 0, 0, time.UTC)

	p1 := newProcess(1)
	p2 := newProcess(2)
	p3 := newProcess(3)
	processes := []*table.Process{p1, p2, p3}

	// p1 有两个实例更新、p2 有一个实例更新；p3 没有任何实例更新
	updated := []*table.ProcessInstance{
		newInst(11, 1),
		newInst(12, 1),
		newInst(21, 2),
	}

	got := collectSyncedProcesses(processes, updated, syncedAt)

	if len(got) != 2 {
		t.Fatalf("synced processes = %d, want 2", len(got))
	}

	gotIDs := map[uint32]bool{}
	for _, p := range got {
		gotIDs[p.ID] = true
		if p.Spec.ProcessStateSyncedAt == nil {
			t.Errorf("process %d ProcessStateSyncedAt should be set", p.ID)
			continue
		}
		if !p.Spec.ProcessStateSyncedAt.Equal(syncedAt) {
			t.Errorf("process %d synced at = %v, want %v", p.ID, *p.Spec.ProcessStateSyncedAt, syncedAt)
		}
	}

	if !gotIDs[1] || !gotIDs[2] {
		t.Errorf("expected processes 1 and 2 to be marked, got %v", gotIDs)
	}
	if gotIDs[3] {
		t.Error("process 3 has no updated instance and must not be marked")
	}
	if p3.Spec.ProcessStateSyncedAt != nil {
		t.Error("process 3 ProcessStateSyncedAt must remain nil")
	}
}

// TestBuildBatchInstMapDuplicateKey 同 host+alias 冲突进程的实例会算出相同 key，
// 多值 map 必须保留全部实例，不能被覆盖。
func TestBuildBatchInstMapDuplicateKey(t *testing.T) {
	const dupKey = "agent-1:GSEKIT_BIZ_2:proc_1"

	inst1 := newInst(11, 100)
	inst2 := newInst(22, 200)
	batch := []bizOperateItem{
		{key: dupKey, inst: inst1},
		{key: dupKey, inst: inst2},
	}

	req, instMap := buildBatchInstMap(batch)

	if len(req) != 2 {
		t.Fatalf("req size = %d, want 2", len(req))
	}
	if got := len(instMap[dupKey]); got != 2 {
		t.Fatalf("instMap[%q] size = %d, want 2 (must not overwrite)", dupKey, got)
	}
}

// TestApplyBatchResultFanOut 单个 GSE 结果必须扇出到同 key 的所有实例，全部更新为同一状态。
func TestApplyBatchResultFanOut(t *testing.T) {
	const dupKey = "agent-1:GSEKIT_BIZ_2:proc_1"

	inst1 := newInst(11, 100)
	inst2 := newInst(22, 200)
	instMap := map[string][]*table.ProcessInstance{
		dupKey: {inst1, inst2},
	}

	// ErrCodeSuccess + 含 pid>0 且 isAuto 的 content => Running + Managed
	result := map[string]gse.ProcResult{
		dupKey: {
			ErrorCode: gse.ErrCodeSuccess,
			Content:   `{"process":[{"instance":[{"pid":1234,"isAuto":true}]}]}`,
		},
	}

	updated := applyBatchResult(result, instMap)

	if len(updated) != 2 {
		t.Fatalf("updated insts = %d, want 2 (both instances of duplicate key)", len(updated))
	}
	for _, inst := range []*table.ProcessInstance{inst1, inst2} {
		if inst.Spec.Status != table.ProcessStatusRunning {
			t.Errorf("inst %d status = %q, want %q", inst.ID, inst.Spec.Status, table.ProcessStatusRunning)
		}
		if inst.Spec.ManagedStatus != table.ProcessManagedStatusManaged {
			t.Errorf("inst %d managed = %q, want %q", inst.ID, inst.Spec.ManagedStatus, table.ProcessManagedStatusManaged)
		}
	}
}

// TestApplyBatchResultInProgressSkipped GSE 仍在执行中(115)的结果不得覆盖实例状态。
// 复现 bug：进程在 GSE 侧已托管/运行，但同步任务还没跑完(115)就被当成完成，
// 误写成 stopped/unmanaged。修复后这类条目应被跳过，保留实例原状态。
func TestApplyBatchResultInProgressSkipped(t *testing.T) {
	const key = "agent-1:GSEKIT_BIZ_3:proc_1"

	inst := newInst(1, 100)
	inst.Spec.Status = table.ProcessStatusRunning
	inst.Spec.ManagedStatus = table.ProcessManagedStatusManaged

	instMap := map[string][]*table.ProcessInstance{key: {inst}}
	result := map[string]gse.ProcResult{
		key: {ErrorCode: gse.ErrCodeInProgress, ErrorMsg: "handling"},
	}

	if updated := applyBatchResult(result, instMap); len(updated) != 0 {
		t.Fatalf("updated insts = %d, want 0 for in-progress result", len(updated))
	}
	if inst.Spec.Status != table.ProcessStatusRunning {
		t.Errorf("inst status = %q, want %q (must not be overwritten while in-progress)",
			inst.Spec.Status, table.ProcessStatusRunning)
	}
	if inst.Spec.ManagedStatus != table.ProcessManagedStatusManaged {
		t.Errorf("inst managed = %q, want %q (must not be overwritten while in-progress)",
			inst.Spec.ManagedStatus, table.ProcessManagedStatusManaged)
	}
}

// TestApplyBatchResultUnknownKeySkipped 结果中没有对应实例的 key 应被跳过。
func TestApplyBatchResultUnknownKeySkipped(t *testing.T) {
	instMap := map[string][]*table.ProcessInstance{
		"known": {newInst(1, 10)},
	}
	result := map[string]gse.ProcResult{
		"unknown": {ErrorCode: gse.ErrCodeSuccess, Content: `{"process":[]}`},
	}

	if updated := applyBatchResult(result, instMap); len(updated) != 0 {
		t.Fatalf("updated insts = %d, want 0 for unmatched key", len(updated))
	}
}

// TestCollectSyncedProcessesEmpty 没有任何实例更新时返回空集合。
func TestCollectSyncedProcessesEmpty(t *testing.T) {
	processes := []*table.Process{newProcess(1), newProcess(2)}

	got := collectSyncedProcesses(processes, nil, time.Now().UTC())
	if len(got) != 0 {
		t.Fatalf("synced processes = %d, want 0", len(got))
	}
	for _, p := range processes {
		if p.Spec.ProcessStateSyncedAt != nil {
			t.Errorf("process %d ProcessStateSyncedAt must remain nil", p.ID)
		}
	}
}
