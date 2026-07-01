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
	"strings"
	"testing"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

// nginx1Actual 基准 nginx_1 实际项（与 samples/proc-example.json 一致）。
func nginx1Actual() ActualProc {
	return ActualProc{
		Contact:    "GSEKIT_BIZ_100148",
		ValueKey:   "GSEKIT_BIZ_100148:nginx_1",
		ProcName:   "nginx",
		SetupPath:  "/usr/sbin",
		PidPath:    "/run/nginx-1.pid",
		User:       "root",
		StartCmd:   "nginx -c /etc/nginx/nginx-1.conf",
		StopCmd:    "nginx -c /etc/nginx/nginx-1.conf -s stop",
		RestartCmd: "nginx -c /etc/nginx/nginx-1.conf -s reload",
		ReloadCmd:  "nginx -c /etc/nginx/nginx-1.conf -s reload",
		KillCmd:    "kill -9 $(cat /run/nginx-1.pid)",
	}
}

// nginx1Expected 与 nginx1Actual 9 字段一致的期望项。
func nginx1Expected(status table.ProcessManagedStatus) ExpectedProc {
	a := nginx1Actual()
	return ExpectedProc{
		ValueKey:          a.ValueKey,
		ManagedStatus:     status,
		ProcName:          a.ProcName,
		SetupPath:         a.SetupPath,
		PidPath:           a.PidPath,
		User:              a.User,
		StartCmd:          a.StartCmd,
		StopCmd:           a.StopCmd,
		RestartCmd:        a.RestartCmd,
		ReloadCmd:         a.ReloadCmd,
		KillCmd:           a.KillCmd,
		ProcessInstanceID: 1,
		ProcessID:         11,
		HostID:            101,
		BizID:             sampleBizID,
		TenantID:          "t1",
		AgentID:           "agent-1",
	}
}

func TestCompareHost_IllegalValueKey(t *testing.T) {
	exp := []ExpectedProc{nginx1Expected(table.ProcessManagedStatusManaged)}
	// actual 多出一个期望集合外的 valuekey。
	actual := []ActualProc{nginx1Actual(), {ValueKey: "GSEKIT_BIZ_100148:ghost_9", Contact: "GSEKIT_BIZ_100148"}}

	got := CompareHost(exp, actual, time.Now())
	if len(got) != 1 {
		t.Fatalf("want 1 result, got %d", len(got))
	}
	if got[0].Verdict != VerdictException || got[0].ErrorType != table.ProcessExceptionIllegalValueKey {
		t.Fatalf("want ILLEGAL_VALUE_KEY exception, got %+v", got[0])
	}
	if !strings.Contains(got[0].ErrorMsg, "GSEKIT_BIZ_100148:ghost_9") {
		t.Fatalf("error msg should contain illegal key, got %s", got[0].ErrorMsg)
	}
}

func TestCompareHost_ManagedMissingActual(t *testing.T) {
	exp := []ExpectedProc{nginx1Expected(table.ProcessManagedStatusManaged)}
	got := CompareHost(exp, nil, time.Now())
	if got[0].Verdict != VerdictException || got[0].ErrorType != table.ProcessExceptionExpectationMismatch {
		t.Fatalf("managed+no-actual want EXPECTATION_MISMATCH, got %+v", got[0])
	}
}

func TestCompareHost_UnmanagedHasActual(t *testing.T) {
	for _, status := range []table.ProcessManagedStatus{table.ProcessManagedStatusUnmanaged, ""} {
		exp := []ExpectedProc{nginx1Expected(status)}
		got := CompareHost(exp, []ActualProc{nginx1Actual()}, time.Now())
		if got[0].Verdict != VerdictException || got[0].ErrorType != table.ProcessExceptionExpectationMismatch {
			t.Fatalf("unmanaged(%q)+actual want EXPECTATION_MISMATCH, got %+v", status, got[0])
		}
	}
}

func TestCompareHost_UnmanagedNoActual(t *testing.T) {
	for _, status := range []table.ProcessManagedStatus{table.ProcessManagedStatusUnmanaged, ""} {
		exp := []ExpectedProc{nginx1Expected(status)}
		got := CompareHost(exp, nil, time.Now())
		if got[0].Verdict != VerdictPass {
			t.Fatalf("unmanaged(%q)+no-actual want pass, got %+v", status, got[0])
		}
	}
}

func TestCompareHost_TransitionSkip(t *testing.T) {
	for _, status := range []table.ProcessManagedStatus{
		table.ProcessManagedStatusStarting, table.ProcessManagedStatusStopping,
	} {
		exp := []ExpectedProc{nginx1Expected(status)}
		// 即便 actual 存在也应跳过。
		got := CompareHost(exp, []ActualProc{nginx1Actual()}, time.Now())
		if got[0].Verdict != VerdictSkip {
			t.Fatalf("status(%q) want skip, got %+v", status, got[0])
		}
	}
}

func TestCompareHost_PartlyManagedSkip(t *testing.T) {
	exp := []ExpectedProc{nginx1Expected(table.ProcessManagedStatusPartlyManaged)}
	got := CompareHost(exp, []ActualProc{nginx1Actual()}, time.Now())
	if got[0].Verdict != VerdictSkip {
		t.Fatalf("partly_managed want skip, got %+v", got[0])
	}
}

func TestCompareHost_ManagedConsistentPass(t *testing.T) {
	exp := []ExpectedProc{nginx1Expected(table.ProcessManagedStatusManaged)}
	got := CompareHost(exp, []ActualProc{nginx1Actual()}, time.Now())
	if got[0].Verdict != VerdictPass {
		t.Fatalf("managed+consistent want pass, got %+v", got[0])
	}
}

func TestCompareHost_ManagedFieldDiff(t *testing.T) {
	exp := []ExpectedProc{nginx1Expected(table.ProcessManagedStatusManaged)}
	actual := nginx1Actual()
	actual.StartCmd = "nginx -c /etc/nginx/DRIFTED.conf"
	actual.User = "nobody"

	got := CompareHost(exp, []ActualProc{actual}, time.Now())
	if got[0].Verdict != VerdictException || got[0].ErrorType != table.ProcessExceptionExpectationMismatch {
		t.Fatalf("field diff want EXPECTATION_MISMATCH, got %+v", got[0])
	}
	if !strings.Contains(got[0].ErrorMsg, "startCmd") || !strings.Contains(got[0].ErrorMsg, "user") {
		t.Fatalf("error msg should list diff fields, got %s", got[0].ErrorMsg)
	}
}
