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
	"strings"
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

func TestCascadeBlockMessage(t *testing.T) {
	t.Parallel()

	asc := CascadeBlockMessage(100, PriorityOrderAsc)
	if !strings.Contains(asc, "优先级大于此的进程操作不会被继续执行") {
		t.Fatalf("升序阻断文案不符合预期: %s", asc)
	}

	desc := CascadeBlockMessage(100, PriorityOrderDesc)
	if !strings.Contains(desc, "优先级小于此的进程操作不会被继续执行") {
		t.Fatalf("降序阻断文案不符合预期: %s", desc)
	}
}

func TestTaskBatchExtraDataRoundTrip(t *testing.T) {
	t.Parallel()

	spec := &TaskBatchSpec{}
	if err := spec.SetExtraData(&TaskBatchExtraData{
		GroupID:         "group-1",
		RegisterProcess: &RegisterProcessExtra{SuccessCount: 3},
	}); err != nil {
		t.Fatalf("SetExtraData 失败: %v", err)
	}

	got, err := spec.GetExtraData()
	if err != nil {
		t.Fatalf("GetExtraData 失败: %v", err)
	}
	if got.GroupID != "group-1" {
		t.Fatalf("GroupID 丢失: %+v", got)
	}
	if got.RegisterProcess == nil || got.RegisterProcess.SuccessCount != 3 {
		t.Fatalf("RegisterProcess 丢失: %+v", got.RegisterProcess)
	}
}

func TestGetExtraDataOnEmpty(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "{}"} {
		spec := &TaskBatchSpec{ExtraData: raw}
		got, err := spec.GetExtraData()
		if err != nil {
			t.Fatalf("GetExtraData(%q) 失败: %v", raw, err)
		}
		if got == nil || got.GroupID != "" {
			t.Fatalf("GetExtraData(%q) 应返回空结构, got %+v", raw, got)
		}
	}
}
