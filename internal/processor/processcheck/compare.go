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

package processcheck

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

// Verdict 单实例检查结论。
type Verdict string

const (
	// VerdictException 异常：需写入一条 exception 记录。
	VerdictException Verdict = "exception"
	// VerdictPass 通过：本轮一致，可触发恢复闭环（最新记录为 exception 时翻转 recovered）。
	VerdictPass Verdict = "pass"
	// VerdictSkip 跳过：过渡态/忽略，无任何写入或状态更新。
	VerdictSkip Verdict = "skip"
)

// 处理建议文案对标 gsekit check_process.py。
const (
	suggestionIllegalValueKey  = "请检查进程托管配置, 可尝试取消托管再重新托管, 如问题依然存在, 请联系管理员确认"
	suggestionUnmanagedHasInfo = "请检查进程托管配置, 可尝试取消托管再重新托管"
	suggestionManagedNoInfo    = "请检查进程托管配置, 可尝试重新托管"
	suggestionMismatch         = "请检查进程托管配置, 可尝试取消托管再重新托管"
	suggestionParsingFailed    = "请根据错误信息提示进行相应处理"
	suggestionAgentException   = "请检查Agent是否正常运行"
)

// CheckResult 单进程实例的检查结论（运行态，无持久化）。
type CheckResult struct {
	ProcessInstanceID uint32
	ProcessID         uint32
	HostID            uint32
	BizID             uint32
	TenantID          string

	Verdict            Verdict
	ErrorType          table.ProcessExceptionErrorType
	ErrorMsg           string
	HandlingSuggestion string
	CheckedAt          time.Time
}

// comparedField 描述参与子集比对的 9 个字段（驼峰 key 用于差异输出）。
type comparedField struct {
	name     string
	expected func(ExpectedProc) string
	actual   func(ActualProc) string
}

var comparedFields = []comparedField{
	{"procName", func(e ExpectedProc) string { return e.ProcName }, func(a ActualProc) string { return a.ProcName }},
	{"setupPath", func(e ExpectedProc) string { return e.SetupPath }, func(a ActualProc) string { return a.SetupPath }},
	{"pidPath", func(e ExpectedProc) string { return e.PidPath }, func(a ActualProc) string { return a.PidPath }},
	{"user", func(e ExpectedProc) string { return e.User }, func(a ActualProc) string { return a.User }},
	{"startCmd", func(e ExpectedProc) string { return e.StartCmd }, func(a ActualProc) string { return a.StartCmd }},
	{"stopCmd", func(e ExpectedProc) string { return e.StopCmd }, func(a ActualProc) string { return a.StopCmd }},
	{"restartCmd", func(e ExpectedProc) string { return e.RestartCmd }, func(a ActualProc) string { return a.RestartCmd }},
	{"reloadCmd", func(e ExpectedProc) string { return e.ReloadCmd }, func(a ActualProc) string { return a.ReloadCmd }},
	{"killCmd", func(e ExpectedProc) string { return e.KillCmd }, func(a ActualProc) string { return a.KillCmd }},
}

// CompareHost 对单个 agent(host) 的期望项与实际项做比对，返回每个期望实例的结论。
// 前提：.proc 已解析成功（解析失败/agent 异常为 host 级错误，由编排层经 HostError 扇出，不进入此函数）。
// 规则对标 gsekit _check_process_mismatch + _check_single_proc：
//  1. host 级非法 valuekey（actual_keys - expected_keys 非空）→ 该 host 全部实例记 ILLEGAL_VALUE_KEY 并短路；
//  2. 否则逐实例按 ManagedStatus 分支，managed 且 actual 存在时做 9 字段子集比对。
func CompareHost(expected []ExpectedProc, actual []ActualProc, checkedAt time.Time) []CheckResult {
	actualByKey := make(map[string]ActualProc, len(actual))
	for _, a := range actual {
		actualByKey[a.ValueKey] = a
	}
	expectedKeys := make(map[string]struct{}, len(expected))
	for _, e := range expected {
		expectedKeys[e.ValueKey] = struct{}{}
	}

	illegal := make([]string, 0)
	for _, a := range actual {
		if _, ok := expectedKeys[a.ValueKey]; !ok {
			illegal = append(illegal, a.ValueKey)
		}
	}

	results := make([]CheckResult, 0, len(expected))

	// host 级非法 valuekey 短路：全部实例记 ILLEGAL_VALUE_KEY（对标 gsekit illegal_keys 后 continue）。
	if len(illegal) > 0 {
		sort.Strings(illegal)
		msg := fmt.Sprintf("存在非法valuekey托管信息: %v", illegal)
		for _, e := range expected {
			results = append(results, exceptionResult(e, table.ProcessExceptionIllegalValueKey, msg,
				suggestionIllegalValueKey, checkedAt))
		}
		return results
	}

	for _, e := range expected {
		results = append(results, checkSingle(e, actualByKey, checkedAt))
	}
	return results
}

// checkSingle 以 ManagedStatus 为基准判定单实例。
func checkSingle(e ExpectedProc, actualByKey map[string]ActualProc, checkedAt time.Time) CheckResult {
	actual, has := actualByKey[e.ValueKey]

	switch e.ManagedStatus {
	case table.ProcessManagedStatusStarting, table.ProcessManagedStatusStopping,
		table.ProcessManagedStatusPartlyManaged:
		// 操作过渡态/部分托管：本轮跳过，避免操作窗口误报。
		return skipResult(e, checkedAt)

	case table.ProcessManagedStatusManaged:
		if !has {
			return exceptionResult(e, table.ProcessExceptionExpectationMismatch,
				fmt.Sprintf("进程已托管但未获取到信息: %s", e.ValueKey), suggestionManagedNoInfo, checkedAt)
		}
		if diffs := diffFields(e, actual); len(diffs) > 0 {
			return exceptionResult(e, table.ProcessExceptionExpectationMismatch,
				fmt.Sprintf("托管信息与预期不符, 差异字段: [%s]", strings.Join(diffs, ", ")),
				suggestionMismatch, checkedAt)
		}
		return passResult(e, checkedAt)

	case table.ProcessManagedStatusUnmanaged, "":
		// 不应托管：存在 actual → 异常；不存在 → 通过（不记录新异常，但可触发恢复闭环）。
		if has {
			return exceptionResult(e, table.ProcessExceptionExpectationMismatch,
				fmt.Sprintf("进程未托管但获取到信息: %s", e.ValueKey), suggestionUnmanagedHasInfo, checkedAt)
		}
		return passResult(e, checkedAt)

	default:
		// 未知托管态：忽略，避免误报。
		return skipResult(e, checkedAt)
	}
}

// diffFields 返回 9 字段中期望与实际不一致的字段名集合（对标 gsekit proc.items() <= actual.items() 的失败项）。
func diffFields(e ExpectedProc, a ActualProc) []string {
	diffs := make([]string, 0)
	for _, f := range comparedFields {
		if f.expected(e) != f.actual(a) {
			diffs = append(diffs, f.name)
		}
	}
	return diffs
}

// HostError 把 host 级错误（解析失败/agent 异常）扇出到该 host 下全部相关实例（FR-010）。
func HostError(expected []ExpectedProc, errType table.ProcessExceptionErrorType,
	errMsg, suggestion string, checkedAt time.Time) []CheckResult {
	results := make([]CheckResult, 0, len(expected))
	for _, e := range expected {
		results = append(results, exceptionResult(e, errType, errMsg, suggestion, checkedAt))
	}
	return results
}

func exceptionResult(e ExpectedProc, errType table.ProcessExceptionErrorType,
	errMsg, suggestion string, checkedAt time.Time) CheckResult {
	r := baseResult(e, checkedAt)
	r.Verdict = VerdictException
	r.ErrorType = errType
	r.ErrorMsg = errMsg
	r.HandlingSuggestion = suggestion
	return r
}

func passResult(e ExpectedProc, checkedAt time.Time) CheckResult {
	r := baseResult(e, checkedAt)
	r.Verdict = VerdictPass
	return r
}

func skipResult(e ExpectedProc, checkedAt time.Time) CheckResult {
	r := baseResult(e, checkedAt)
	r.Verdict = VerdictSkip
	return r
}

func baseResult(e ExpectedProc, checkedAt time.Time) CheckResult {
	return CheckResult{
		ProcessInstanceID: e.ProcessInstanceID,
		ProcessID:         e.ProcessID,
		HostID:            e.HostID,
		BizID:             e.BizID,
		TenantID:          e.TenantID,
		CheckedAt:         checkedAt,
	}
}
