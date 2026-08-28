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

package crontab

import (
	"reflect"
	"testing"
)

func TestNextBizBatch(t *testing.T) {
	bizIDs := []int{1, 3, 5, 7, 9}

	t.Run("start from beginning", func(t *testing.T) {
		batch, last := nextBizBatch(bizIDs, 0, 2)
		if !reflect.DeepEqual(batch, []int{1, 3}) {
			t.Fatalf("batch = %v, want [1 3]", batch)
		}
		if last != 3 {
			t.Fatalf("last = %d, want 3", last)
		}
	})

	t.Run("continue after cursor", func(t *testing.T) {
		batch, last := nextBizBatch(bizIDs, 3, 2)
		if !reflect.DeepEqual(batch, []int{5, 7}) {
			t.Fatalf("batch = %v, want [5 7]", batch)
		}
		if last != 7 {
			t.Fatalf("last = %d, want 7", last)
		}
	})

	t.Run("wrap around", func(t *testing.T) {
		batch, last := nextBizBatch(bizIDs, 9, 2)
		if !reflect.DeepEqual(batch, []int{1, 3}) {
			t.Fatalf("batch = %v, want [1 3]", batch)
		}
		if last != 3 {
			t.Fatalf("last = %d, want 3", last)
		}
	})

	t.Run("wrap with remainder", func(t *testing.T) {
		batch, last := nextBizBatch(bizIDs, 7, 3)
		if !reflect.DeepEqual(batch, []int{9, 1, 3}) {
			t.Fatalf("batch = %v, want [9 1 3]", batch)
		}
		if last != 3 {
			t.Fatalf("last = %d, want 3", last)
		}
	})

	t.Run("removed cursor biz still advances", func(t *testing.T) {
		batch, last := nextBizBatch(bizIDs, 4, 2)
		if !reflect.DeepEqual(batch, []int{5, 7}) {
			t.Fatalf("batch = %v, want [5 7]", batch)
		}
		if last != 7 {
			t.Fatalf("last = %d, want 7", last)
		}
	})

	t.Run("batch larger than list", func(t *testing.T) {
		batch, last := nextBizBatch(bizIDs, 0, 10)
		if !reflect.DeepEqual(batch, bizIDs) {
			t.Fatalf("batch = %v, want %v", batch, bizIDs)
		}
		if last != 9 {
			t.Fatalf("last = %d, want 9", last)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		batch, last := nextBizBatch(nil, 5, 10)
		if len(batch) != 0 {
			t.Fatalf("batch = %v, want empty", batch)
		}
		if last != 5 {
			t.Fatalf("last = %d, want 5", last)
		}
	})
}

func TestSyncCmdbGseBizCursorConfigKey(t *testing.T) {
	if got := syncCmdbGseBizCursorConfigKey(""); got != "sync_cmdb_gse_biz_cursor:default" {
		t.Fatalf("empty tenant key = %q", got)
	}
	if got := syncCmdbGseBizCursorConfigKey("t1"); got != "sync_cmdb_gse_biz_cursor:t1" {
		t.Fatalf("tenant key = %q", got)
	}
}
