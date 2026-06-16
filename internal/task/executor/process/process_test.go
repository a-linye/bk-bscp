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
		wantAlreadyRun bool
	}{
		{
			name:           "start but gse running and bscp running -> already running",
			operateType:    table.StartProcessOperate,
			content:        statusContent(100, true),
			origStatus:     table.ProcessStatusRunning,
			origManaged:    table.ProcessManagedStatusManaged,
			wantValid:      false,
			wantAlreadyRun: true,
		},
		{
			name:           "start but gse running while bscp stopped -> status mismatch but already running",
			operateType:    table.StartProcessOperate,
			content:        statusContent(100, true),
			origStatus:     table.ProcessStatusStopped,
			origManaged:    table.ProcessManagedStatusUnmanaged,
			wantValid:      false,
			wantAlreadyRun: true,
		},
		{
			name:           "start and gse stopped -> valid",
			operateType:    table.StartProcessOperate,
			content:        statusContent(0, false),
			origStatus:     table.ProcessStatusStopped,
			origManaged:    table.ProcessManagedStatusUnmanaged,
			wantValid:      true,
			wantAlreadyRun: false,
		},
		{
			name:           "stop and gse stopped -> invalid, not already running",
			operateType:    table.StopProcessOperate,
			content:        statusContent(0, false),
			origStatus:     table.ProcessStatusStopped,
			origManaged:    table.ProcessManagedStatusUnmanaged,
			wantValid:      false,
			wantAlreadyRun: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotValid, gotAlreadyRun, _ := isOperationValid(c.operateType, c.content, c.origStatus, c.origManaged)
			if gotValid != c.wantValid {
				t.Fatalf("isValid = %v, want %v", gotValid, c.wantValid)
			}
			if gotAlreadyRun != c.wantAlreadyRun {
				t.Fatalf("alreadyRunning = %v, want %v", gotAlreadyRun, c.wantAlreadyRun)
			}
		})
	}
}
