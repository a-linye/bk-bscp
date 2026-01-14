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

package config

import (
	"testing"

	"github.com/TencentBlueKing/bk-bscp/internal/task/executor/common"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// TestRenderFileNameAndPath 测试文件名和路径渲染功能
func TestRenderFileNameAndPath(t *testing.T) {
	// 创建 executor 实例
	executor := &PushConfigExecutor{}

	tests := []struct {
		name        string
		payload     *common.TaskPayload
		wantName    string
		wantPath    string
		wantErr     bool
		description string
	}{
		{
			name: "no variables in filename and path",
			payload: &common.TaskPayload{
				ProcessPayload: &common.ProcessPayload{
					SetName:       "test-set",
					ModuleName:    "test-module",
					ServiceName:   "test-service",
					Alias:         "test-process",
					FuncName:      "test-func",
					InnerIP:       "127.0.0.1",
					CcProcessID:   1001,
					HostInstSeq:   1,
					ModuleInstSeq: 1,
					CloudID:       0,
				},
				ConfigPayload: &common.ConfigPayload{
					ConfigFileName: "config.yaml",
					ConfigFilePath: "/etc/app",
				},
			},
			wantName:    "config.yaml",
			wantPath:    "/etc/app",
			wantErr:     false,
			description: "测试不包含变量的文件名和路径",
		},
		{
			name: "variables in filename",
			payload: &common.TaskPayload{
				ProcessPayload: &common.ProcessPayload{
					SetName:       "test-set",
					ModuleName:    "test-module",
					ServiceName:   "test-service",
					Alias:         "test-process",
					FuncName:      "test-func",
					InnerIP:       "127.0.0.1",
					CcProcessID:   1001,
					HostInstSeq:   1,
					ModuleInstSeq: 2,
					CloudID:       0,
				},
				ConfigPayload: &common.ConfigPayload{
					ConfigFileName: "config_${inst_id}.yaml",
					ConfigFilePath: "/etc/app",
				},
			},
			wantName:    "config_2.yaml",
			wantPath:    "/etc/app",
			wantErr:     false,
			description: "测试文件名包含 inst_id 变量",
		},
		{
			name: "variables in filepath",
			payload: &common.TaskPayload{
				ProcessPayload: &common.ProcessPayload{
					SetName:       "test-set",
					ModuleName:    "test-module",
					ServiceName:   "test-service",
					Alias:         "test-process",
					FuncName:      "test-func",
					InnerIP:       "127.0.0.1",
					CcProcessID:   1001,
					HostInstSeq:   1,
					ModuleInstSeq: 3,
					CloudID:       0,
				},
				ConfigPayload: &common.ConfigPayload{
					ConfigFileName: "config.yaml",
					ConfigFilePath: "/etc/${bk_module_name}",
				},
			},
			wantName:    "config.yaml",
			wantPath:    "/etc/test-module",
			wantErr:     false,
			description: "测试文件路径包含 bk_module_name 变量",
		},
		{
			name: "variables in both filename and filepath",
			payload: &common.TaskPayload{
				ProcessPayload: &common.ProcessPayload{
					SetName:       "prod-set",
					ModuleName:    "web-module",
					ServiceName:   "nginx-service",
					Alias:         "nginx",
					FuncName:      "nginx-bin",
					InnerIP:       "10.0.0.1",
					CcProcessID:   2001,
					HostInstSeq:   5,
					ModuleInstSeq: 10,
					CloudID:       1,
				},
				ConfigPayload: &common.ConfigPayload{
					ConfigFileName: "${bk_process_name}_${inst_id}.conf",
					ConfigFilePath: "/etc/${bk_set_name}/${bk_module_name}",
				},
			},
			wantName:    "nginx_10.conf",
			wantPath:    "/etc/prod-set/web-module",
			wantErr:     false,
			description: "测试文件名和路径都包含变量",
		},
		{
			name: "multiple instances with different inst_id",
			payload: &common.TaskPayload{
				ProcessPayload: &common.ProcessPayload{
					SetName:       "app-set",
					ModuleName:    "backend-module",
					ServiceName:   "api-service",
					Alias:         "api-server",
					FuncName:      "api",
					InnerIP:       "192.168.1.100",
					CcProcessID:   3001,
					HostInstSeq:   2,
					ModuleInstSeq: 15,
					CloudID:       0,
				},
				ConfigPayload: &common.ConfigPayload{
					ConfigFileName: "app_${local_inst_id}_inst${inst_id}.json",
					ConfigFilePath: "/data/${bk_host_innerip}",
				},
			},
			wantName:    "app_2_inst15.json",
			wantPath:    "/data/192.168.1.100",
			wantErr:     false,
			description: "测试使用 inst_id 和 local_inst_id 区分多实例",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("测试用例: %s", tt.description)

			kt := kit.New()
			gotName, gotPath, err := executor.renderFileNameAndPath(kt, tt.payload)

			if (err != nil) != tt.wantErr {
				t.Errorf("renderFileNameAndPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotName != tt.wantName {
				t.Errorf("renderFileNameAndPath() gotName = %v, want %v", gotName, tt.wantName)
			}

			if gotPath != tt.wantPath {
				t.Errorf("renderFileNameAndPath() gotPath = %v, want %v", gotPath, tt.wantPath)
			}

			t.Logf("渲染结果: %s -> %s, %s -> %s",
				tt.payload.ConfigPayload.ConfigFileName, gotName,
				tt.payload.ConfigPayload.ConfigFilePath, gotPath)
		})
	}
}
