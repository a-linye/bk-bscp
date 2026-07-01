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
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// fakeRunner 按 agentID 返回预置 Screen / 错误。
type fakeRunner struct {
	screens map[string]string
	errs    map[string]error
}

func (f *fakeRunner) RunProcScript(_ context.Context, agentID, _ string) (string, error) {
	if f.errs != nil {
		if e := f.errs[agentID]; e != nil {
			return "", e
		}
	}
	return f.screens[agentID], nil
}

func screenFromActual(procs ...ActualProc) string {
	b, _ := json.Marshal(map[string][]ActualProc{"proc": procs})
	return string(b)
}

func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

// TestRunChecks_DriftWritesException 差异实例写一条 exception（AC-001/SC-001）。
func TestRunChecks_DriftWritesException(t *testing.T) {
	exp := nginx1Expected(table.ProcessManagedStatusManaged)
	exp.AgentID = "agent-1"

	drift := nginx1Actual()
	drift.StartCmd = "nginx -c /etc/nginx/DRIFTED.conf"

	runner := &fakeRunner{screens: map[string]string{"agent-1": screenFromActual(drift)}}
	store := newFakeStore()

	runChecks(kit.New(), sampleBizID, []ExpectedProc{exp}, runner, store, noLimit(), time.Now())

	if len(store.created) != 1 {
		t.Fatalf("want 1 exception record, got %d", len(store.created))
	}
	if store.created[0].Spec.ErrorType != table.ProcessExceptionExpectationMismatch {
		t.Fatalf("want EXPECTATION_MISMATCH, got %s", store.created[0].Spec.ErrorType)
	}
}

// TestRunChecks_ParseFailureIsolated 单主机解析失败记 PARSING_FAILED，其余主机继续完成（AC-002/AC-T02/SC-002/SC-005）。
func TestRunChecks_ParseFailureIsolated(t *testing.T) {
	good := nginx1Expected(table.ProcessManagedStatusManaged)
	good.AgentID = "agent-good"

	bad := nginx1Expected(table.ProcessManagedStatusManaged)
	bad.AgentID = "agent-bad"
	bad.ProcessInstanceID = 2
	bad.ValueKey = "GSEKIT_BIZ_100148:nginx_2"

	runner := &fakeRunner{screens: map[string]string{
		"agent-good": screenFromActual(nginx1Actual()), // 一致 → pass
		"agent-bad":  "command not found",              // 非 JSON → PARSING_FAILED
	}}
	store := newFakeStore()

	runChecks(kit.New(), sampleBizID, []ExpectedProc{good, bad}, runner, store, noLimit(), time.Now())

	if len(store.created) != 1 {
		t.Fatalf("want only bad host record, got %d", len(store.created))
	}
	rec := store.created[0]
	if rec.Spec.ErrorType != table.ProcessExceptionParsingFailed || rec.Attachment.ProcessInstanceID != 2 {
		t.Fatalf("want PARSING_FAILED for instID=2, got %+v", rec)
	}
}

// TestRunChecks_IllegalValueKeyNodemanNotMisreported 非法 valuekey 记 ILLEGAL_VALUE_KEY，nodeman 不误报（AC-T03/SC-006）。
func TestRunChecks_IllegalValueKeyNodemanNotMisreported(t *testing.T) {
	// 期望集合仅有 nginx_1（managed）；agent 实际有 nginx_1/2/3 + 2 条 nodeman。
	exp := nginx1Expected(table.ProcessManagedStatusManaged)
	exp.AgentID = "agent-1"

	runner := &fakeRunner{screens: map[string]string{"agent-1": sampleProcScreen}}
	store := newFakeStore()

	runChecks(kit.New(), sampleBizID, []ExpectedProc{exp}, runner, store, noLimit(), time.Now())

	if len(store.created) != 1 {
		t.Fatalf("want 1 illegal record, got %d", len(store.created))
	}
	rec := store.created[0]
	if rec.Spec.ErrorType != table.ProcessExceptionIllegalValueKey {
		t.Fatalf("want ILLEGAL_VALUE_KEY, got %s", rec.Spec.ErrorType)
	}
	if !strings.Contains(rec.Spec.ErrorMsg, "nginx_2") || !strings.Contains(rec.Spec.ErrorMsg, "nginx_3") {
		t.Fatalf("illegal msg should list nginx_2/nginx_3, got %s", rec.Spec.ErrorMsg)
	}
	if strings.Contains(rec.Spec.ErrorMsg, "nodeman") {
		t.Fatalf("nodeman must not be reported as illegal, got %s", rec.Spec.ErrorMsg)
	}
}

// TestRunChecks_RunnerErrorNoPanic 单主机下发失败不 panic，扇出 PARSING_FAILED（FR-012）。
func TestRunChecks_RunnerErrorNoPanic(t *testing.T) {
	exp := nginx1Expected(table.ProcessManagedStatusManaged)
	exp.AgentID = "agent-err"

	runner := &fakeRunner{errs: map[string]error{"agent-err": context.DeadlineExceeded}}
	store := newFakeStore()

	runChecks(kit.New(), sampleBizID, []ExpectedProc{exp}, runner, store, noLimit(), time.Now())

	if len(store.created) != 1 || store.created[0].Spec.ErrorType != table.ProcessExceptionParsingFailed {
		t.Fatalf("want 1 PARSING_FAILED record, got %+v", store.created)
	}
}

// TestRunChecks_RecoveryClosure 上轮 exception + 本轮一致 → 最新记录 UpdateStatus(recovered)（AC-003/AC-T04/SC-003）。
func TestRunChecks_RecoveryClosure(t *testing.T) {
	exp := nginx1Expected(table.ProcessManagedStatusManaged)
	exp.AgentID = "agent-1"

	runner := &fakeRunner{screens: map[string]string{"agent-1": screenFromActual(nginx1Actual())}}
	store := newFakeStore()
	store.isExceptionRet[exp.ProcessInstanceID] = true
	store.latest[exp.ProcessInstanceID] = &table.ProcessManagedException{
		ID: 88, Spec: &table.ProcessManagedExceptionSpec{Status: table.ProcessExceptionStatusException}}

	runChecks(kit.New(), sampleBizID, []ExpectedProc{exp}, runner, store, noLimit(), time.Now())

	if len(store.updateCalls) != 1 || store.updateCalls[0].id != 88 ||
		store.updateCalls[0].status != table.ProcessExceptionStatusRecovered {
		t.Fatalf("want UpdateStatus(88, recovered), got %+v", store.updateCalls)
	}
	if len(store.created) != 0 {
		t.Fatalf("recovery should not create record")
	}
}

// TestRunChecks_NoRecoveryWhenNotException 本轮一致但最新记录非 exception → 无多余写入。
func TestRunChecks_NoRecoveryWhenNotException(t *testing.T) {
	exp := nginx1Expected(table.ProcessManagedStatusManaged)
	exp.AgentID = "agent-1"

	runner := &fakeRunner{screens: map[string]string{"agent-1": screenFromActual(nginx1Actual())}}
	store := newFakeStore()
	store.isExceptionRet[exp.ProcessInstanceID] = false

	runChecks(kit.New(), sampleBizID, []ExpectedProc{exp}, runner, store, noLimit(), time.Now())

	if len(store.created) != 0 || len(store.updateCalls) != 0 {
		t.Fatalf("consistent + non-exception must be no-op, created=%d update=%d",
			len(store.created), len(store.updateCalls))
	}
}
