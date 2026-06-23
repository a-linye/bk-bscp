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

package itsm

import (
	"testing"

	"github.com/TencentBlueKing/bk-bscp/internal/criteria/constant"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

func TestBuildStateIDKey(t *testing.T) {
	tests := []struct {
		name        string
		tenantID    string
		approveType table.ApproveType
		want        string
	}{
		{
			name:        "空租户不加前缀-或签",
			tenantID:    "",
			approveType: table.OrSign,
			want:        constant.CreateOrSignApproveItsmStateID,
		},
		{
			name:        "空租户不加前缀-会签",
			tenantID:    "",
			approveType: table.CountSign,
			want:        constant.CreateCountSignApproveItsmStateID,
		},
		{
			// 网关在非多租户上云环境默认注入 X-Bk-Tenant-Id: default，
			// 配置写入侧按空租户落库（无前缀），读取侧须把 default 归一化为无租户。
			name:        "default租户归一化为无租户-或签",
			tenantID:    constant.DefaultTenantID,
			approveType: table.OrSign,
			want:        constant.CreateOrSignApproveItsmStateID,
		},
		{
			name:        "default租户归一化为无租户-会签",
			tenantID:    constant.DefaultTenantID,
			approveType: table.CountSign,
			want:        constant.CreateCountSignApproveItsmStateID,
		},
		{
			name:        "真实租户保留前缀-或签",
			tenantID:    "tenant_a",
			approveType: table.OrSign,
			want:        "tenant_a-" + constant.CreateOrSignApproveItsmStateID,
		},
		{
			name:        "真实租户保留前缀-会签",
			tenantID:    "tenant_a",
			approveType: table.CountSign,
			want:        "tenant_a-" + constant.CreateCountSignApproveItsmStateID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildStateIDKey(tt.tenantID, tt.approveType); got != tt.want {
				t.Errorf("BuildStateIDKey(%q, %q) = %q, want %q", tt.tenantID, tt.approveType, got, tt.want)
			}
		})
	}
}

func TestBuildWorkflowIDKey(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		want     string
	}{
		{
			name:     "空租户不加前缀",
			tenantID: "",
			want:     constant.CreateApproveItsmWorkflowID,
		},
		{
			// 与 stateID 同源问题：网关注入的 default 须归一化为无租户，
			// 否则拼出 default-create_approve_itsm_workflow_id 查不到注册时的无前缀 key。
			name:     "default租户归一化为无租户",
			tenantID: constant.DefaultTenantID,
			want:     constant.CreateApproveItsmWorkflowID,
		},
		{
			name:     "真实租户保留前缀",
			tenantID: "tenant_a",
			want:     "tenant_a-" + constant.CreateApproveItsmWorkflowID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildWorkflowIDKey(tt.tenantID); got != tt.want {
				t.Errorf("BuildWorkflowIDKey(%q) = %q, want %q", tt.tenantID, got, tt.want)
			}
		})
	}
}
