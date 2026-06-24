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

package process

import (
	"testing"

	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	pbproc "github.com/TencentBlueKing/bk-bscp/pkg/protocol/core/process"
)

func statusContent(pid int, isAuto bool) *gse.ProcessStatusContent {
	return &gse.ProcessStatusContent{
		Process: []gse.ProcessDetail{
			{Instance: []gse.ProcessInstance{{PID: pid, IsAuto: isAuto}}},
		},
	}
}

func TestIsOperationValid(t *testing.T) {
	cases := []struct {
		name           string
		operateType    table.ProcessOperateType
		content        *gse.ProcessStatusContent
		origStatus     table.ProcessStatus
		origManaged    table.ProcessManagedStatus
		wantValid      bool
		wantIgnoreCode int
	}{
		{
			name:           "start but gse running and bscp running -> already running 828",
			operateType:    table.StartProcessOperate,
			content:        statusContent(100, true),
			origStatus:     table.ProcessStatusRunning,
			origManaged:    table.ProcessManagedStatusManaged,
			wantValid:      false,
			wantIgnoreCode: gse.ErrCodeAlreadyRunning,
		},
		{
			name:           "start but gse running while bscp stopped -> status mismatch but already running 828",
			operateType:    table.StartProcessOperate,
			content:        statusContent(100, true),
			origStatus:     table.ProcessStatusStopped,
			origManaged:    table.ProcessManagedStatusUnmanaged,
			wantValid:      false,
			wantIgnoreCode: gse.ErrCodeAlreadyRunning,
		},
		{
			name:           "start and gse stopped -> valid",
			operateType:    table.StartProcessOperate,
			content:        statusContent(0, false),
			origStatus:     table.ProcessStatusStopped,
			origManaged:    table.ProcessManagedStatusUnmanaged,
			wantValid:      true,
			wantIgnoreCode: 0,
		},
		{
			name:           "stop and gse stopped -> no need stop 829",
			operateType:    table.StopProcessOperate,
			content:        statusContent(0, false),
			origStatus:     table.ProcessStatusStopped,
			origManaged:    table.ProcessManagedStatusUnmanaged,
			wantValid:      false,
			wantIgnoreCode: gse.ErrCodeNoNeedStop,
		},
		{
			name:           "kill and gse stopped -> no need stop 829",
			operateType:    table.KillProcessOperate,
			content:        statusContent(0, false),
			origStatus:     table.ProcessStatusStopped,
			origManaged:    table.ProcessManagedStatusUnmanaged,
			wantValid:      false,
			wantIgnoreCode: gse.ErrCodeNoNeedStop,
		},
		{
			name:           "stop but gse stopped while bscp running -> status mismatch but no need stop 829",
			operateType:    table.StopProcessOperate,
			content:        statusContent(0, false),
			origStatus:     table.ProcessStatusRunning,
			origManaged:    table.ProcessManagedStatusManaged,
			wantValid:      false,
			wantIgnoreCode: gse.ErrCodeNoNeedStop,
		},
		{
			name:           "stop and gse running -> valid",
			operateType:    table.StopProcessOperate,
			content:        statusContent(100, true),
			origStatus:     table.ProcessStatusRunning,
			origManaged:    table.ProcessManagedStatusManaged,
			wantValid:      true,
			wantIgnoreCode: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotValid, gotIgnoreCode, _ := isOperationValid(c.operateType, c.content, c.origStatus, c.origManaged)
			if gotValid != c.wantValid {
				t.Fatalf("isValid = %v, want %v", gotValid, c.wantValid)
			}
			if gotIgnoreCode != c.wantIgnoreCode {
				t.Fatalf("ignoreErrCode = %d, want %d", gotIgnoreCode, c.wantIgnoreCode)
			}
		})
	}
}

func TestValidateOperateIgnoreErrCode(t *testing.T) {
	cases := []struct {
		name        string
		reason      string
		operateType table.ProcessOperateType
		want        int
	}{
		{"no need operate + start -> 828", pbproc.DisableReasonNoNeedOperate, table.StartProcessOperate, gse.ErrCodeAlreadyRunning},
		{"no need operate + stop -> 829", pbproc.DisableReasonNoNeedOperate, table.StopProcessOperate, gse.ErrCodeNoNeedStop},
		{"no need operate + kill -> 829", pbproc.DisableReasonNoNeedOperate, table.KillProcessOperate, gse.ErrCodeNoNeedStop},
		{"no need operate + register -> 0 (out of scope)", pbproc.DisableReasonNoNeedOperate, table.RegisterProcessOperate, 0},
		{"no need operate + unregister -> 0 (out of scope)", pbproc.DisableReasonNoNeedOperate, table.UnregisterProcessOperate, 0},
		{"other reason + start -> 0", pbproc.DisableReasonTaskRunning, table.StartProcessOperate, 0},
		{"no reason + stop -> 0", pbproc.DisableReasonNone, table.StopProcessOperate, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := validateOperateIgnoreErrCode(c.reason, c.operateType); got != c.want {
				t.Fatalf("validateOperateIgnoreErrCode(%q, %q) = %d, want %d", c.reason, c.operateType, got, c.want)
			}
		})
	}
}
