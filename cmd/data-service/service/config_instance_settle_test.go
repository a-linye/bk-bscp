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

package service

import (
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/TencentBlueKing/bk-bscp/internal/i18n/translations"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/i18n"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// TestBatchHasFailedTasks 只有批次全部执行完才能断定失败；
// 未完成时剩余任务仍可能成功，此时应归因为落库延迟而不是失败任务。
func TestBatchHasFailedTasks(t *testing.T) {
	cases := []struct {
		name string
		spec *table.TaskBatchSpec
		want bool
	}{
		{name: "空指针", spec: nil, want: false},
		{
			name: "全部完成且有失败任务",
			spec: &table.TaskBatchSpec{TotalCount: 3, CompletedCount: 3, SuccessCount: 2, FailedCount: 1},
			want: true,
		},
		{
			name: "全部完成且全部成功",
			spec: &table.TaskBatchSpec{TotalCount: 3, CompletedCount: 3, SuccessCount: 3},
			want: false,
		},
		{
			name: "尚未完成时不判定为失败",
			spec: &table.TaskBatchSpec{TotalCount: 3, CompletedCount: 2, SuccessCount: 2},
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := batchHasFailedTasks(c.spec); got != c.want {
				t.Fatalf("batchHasFailedTasks() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestWaitSuccessTasksSettledRejectsEmptyBatch total_count 为 0 时直接报错，不进入重试，
// 因此不会触碰 dao 与 task storage。
func TestWaitSuccessTasksSettledRejectsEmptyBatch(t *testing.T) {
	batch := &table.TaskBatch{ID: 1761, Spec: &table.TaskBatchSpec{TotalCount: 0}}

	start := time.Now()
	tasks, err := waitSuccessTasksSettled(nil, &kit.Kit{Lang: "zh"}, 100148, batch)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error for a batch whose total count is 0")
	}
	if tasks != nil {
		t.Fatalf("expected no task, got %d", len(tasks))
	}
	if elapsed >= pushConfigSettleInterval {
		t.Fatalf("should fail immediately without retrying, elapsed=%s", elapsed)
	}
	if !strings.Contains(err.Error(), "1761") {
		t.Fatalf("error should carry the batch id: %s", err.Error())
	}
}

// TestBatchIsRunning 批次仍在执行与「已终态但个别任务落库慢」是两回事，
// 前者不能归因为落库延迟，否则会把排查引向错误的时序比对。
func TestBatchIsRunning(t *testing.T) {
	cases := []struct {
		name string
		spec *table.TaskBatchSpec
		want bool
	}{
		{name: "空指针", spec: nil, want: false},
		{
			name: "执行中",
			spec: &table.TaskBatchSpec{Status: table.TaskBatchStatusRunning, TotalCount: 3, CompletedCount: 1},
			want: true,
		},
		{
			name: "已成功",
			spec: &table.TaskBatchSpec{Status: table.TaskBatchStatusSucceed, TotalCount: 3, CompletedCount: 3},
			want: false,
		},
		{
			name: "部分失败",
			spec: &table.TaskBatchSpec{Status: table.TaskBatchStatusPartlyFailed, TotalCount: 3, CompletedCount: 3},
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := batchIsRunning(c.spec); got != c.want {
				t.Fatalf("batchIsRunning() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestSettleErrorMessagesKeepRawNumbers 防止批次 ID 与任务计数被 i18n 加上千分位分隔符，
// 1761 渲染成 1,761 会让用户以为是两个批次。
func TestSettleErrorMessagesKeepRawNumbers(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			kt := &kit.Kit{Lang: lang}

			timeout := i18n.T(kt, "the success task count of batch %s does not match its total count "+
				"(settled %s), still inconsistent after waiting %s, config push aborted",
				formatUint32(1761), formatProgress(2000, 3000), (3 * time.Second).String())
			assertKeepsRawNumbers(t, timeout, "1761", "2000/3000")

			failed := i18n.T(kt, "batch %s has %s failed task(s) (success %s), "+
				"please regenerate the config before pushing",
				formatUint32(1761), formatUint32(1000), formatProgress(2000, 3000))
			assertKeepsRawNumbers(t, failed, "1761", "2000/3000", "1000")

			running := i18n.T(kt, "batch %s is still running (completed %s), "+
				"please wait for the config generation to finish before pushing",
				formatUint32(1761), formatProgress(1000, 3000))
			assertKeepsRawNumbers(t, running, "1761", "1000/3000")
		})
	}
}

// TestRunningMessageIsTranslated 「批次执行中」是本次新增的归因文案，
// 中文缺失会让用户看到英文原文，因此单独断言 zh 已落到 catalog 中。
func TestRunningMessageIsTranslated(t *testing.T) {
	msg := i18n.T(&kit.Kit{Lang: "zh"}, "batch %s is still running (completed %s), "+
		"please wait for the config generation to finish before pushing",
		formatUint32(1761), formatProgress(1000, 3000))

	if strings.Contains(msg, "is still running") {
		t.Fatalf("zh translation missing, got the English source: %s", msg)
	}
	if !strings.Contains(msg, "仍在执行中") {
		t.Fatalf("unexpected zh translation: %s", msg)
	}
}

// thousandSeparated 匹配千分位分隔符，如 1,761
var thousandSeparated = regexp.MustCompile(`\d,\d`)

func assertKeepsRawNumbers(t *testing.T, msg string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(msg, want) {
			t.Errorf("提示中缺少 %q: %s", want, msg)
		}
	}
	if thousandSeparated.MatchString(msg) {
		t.Errorf("数字被加了千分位分隔符: %s", msg)
	}
}
