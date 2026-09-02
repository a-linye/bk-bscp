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

package dao

import (
	"strings"
	"testing"

	"gorm.io/gen/field"
)

// TestAliveProcessCondOrGrouping 锁定进程两阶段查询第二阶段的条件语义：
// 「cc_sync_status != 'deleted' OR id IN (...)」必须渲染为整体带括号的 OR 组，
// 作为单个条件参与外层 AND 组合。若丢括号，AND 优先级高于 OR 会把 biz_id 等前置条件
// 卷进 OR 右侧，放大查询范围（对齐 hook.go 中对 field.Or 的同类约束）。
func TestAliveProcessCondOrGrouping(t *testing.T) {
	ccSyncStatus := field.NewString("processes", "cc_sync_status")
	id := field.NewUint32("processes", "id")

	var sb strings.Builder
	field.Or(ccSyncStatus.Neq("deleted"), id.In(uint32(1), uint32(2))).
		Build(&sqlFragmentBuilder{sb: &sb})
	got := sb.String()

	if !strings.HasPrefix(got, "(") || !strings.HasSuffix(got, ")") {
		t.Fatalf("OR 组必须整体带括号，实际片段: %s", got)
	}
	if !strings.Contains(got, "cc_sync_status") {
		t.Fatalf("应包含 cc_sync_status 条件，实际片段: %s", got)
	}
	if !strings.Contains(got, "id IN") {
		t.Fatalf("应包含 id IN 条件，实际片段: %s", got)
	}
}

// TestAliveProcessCondEmptyIDs 锁定阶段一结果为空集的语义：
// 阶段一（运行中/托管中实例的进程 ID）为空时，id IN (空) 渲染为 IN (NULL) 恒为假，
// 第二阶段自然退化为仅保留 cc_sync_status != 'deleted' 的进程，且不产生 SQL 语法错误。
func TestAliveProcessCondEmptyIDs(t *testing.T) {
	id := field.NewUint32("processes", "id")
	var empty []uint32

	var sb strings.Builder
	id.In(empty...).Build(&sqlFragmentBuilder{sb: &sb})
	got := sb.String()

	if !strings.Contains(got, "IN (NULL)") {
		t.Fatalf("空 ID 集应渲染为 IN (NULL)，实际片段: %s", got)
	}
}

// TestRunningOrManagedInstanceCond 锁定阶段一的条件语义：
// 「status = 'running' OR managed_status = 'managed'」渲染为带括号 OR 组，
// 与 tenant_id/biz_id 等前置 AND 条件组合时不会因优先级问题放大扫描范围。
func TestRunningOrManagedInstanceCond(t *testing.T) {
	status := field.NewString("process_instances", "status")
	managedStatus := field.NewString("process_instances", "managed_status")

	var sb strings.Builder
	field.Or(status.Eq("running"), managedStatus.Eq("managed")).
		Build(&sqlFragmentBuilder{sb: &sb})
	got := sb.String()

	if !strings.HasPrefix(got, "(") || !strings.HasSuffix(got, ")") {
		t.Fatalf("OR 组必须整体带括号，实际片段: %s", got)
	}
	if !strings.Contains(got, "status") || !strings.Contains(got, "managed_status") {
		t.Fatalf("应包含 status 与 managed_status 条件，实际片段: %s", got)
	}
}
