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
