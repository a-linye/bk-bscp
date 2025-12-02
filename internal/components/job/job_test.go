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

package job

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// getJobService 从环境变量获取测试配置
func getJobService(t *testing.T) *Service {
	host := os.Getenv("JOB_HOST")
	appCode := os.Getenv("BK_APP_CODE")
	appSecret := os.Getenv("BK_APP_SECRET")

	if host == "" || appCode == "" || appSecret == "" {
		t.Skip("Skipping test: JOB_HOST, BK_APP_CODE, or BK_APP_SECRET environment variables not set")
	}

	return NewService(appCode, appSecret, host)
}

// TestPushConfigFile 测试分发配置文件接口
func TestPushConfigFile(t *testing.T) {
	jobService := getJobService(t)

	ctx := context.Background()

	// 构造测试请求
	testContent := "test file content"
	encodedContent := base64.StdEncoding.EncodeToString([]byte(testContent))

	req := &PushConfigFileReq{
		BkScopeType:    ScopeTypeBiz,
		BkScopeID:      "1",
		TaskName:       "test-push-config",
		Account:        "root",
		FileTargetPath: "/tmp/test",
		FileList: []FileItem{
			{
				FileName: "test.txt",
				Content:  encodedContent,
			},
		},
		TargetServer: &TargetServer{
			HostIDList: []uint32{1},
		},
	}

	// 执行测试
	resp, err := jobService.PushConfigFile(ctx, req)

	// 断言
	require.NoError(t, err, "PushConfigFile should not return error")
	require.NotNil(t, resp, "Response should not be nil")
	require.NotEmpty(t, resp.JobInstanceID, "JobInstanceID should not be empty")
	require.NotEmpty(t, resp.StepInstanceID, "StepInstanceID should not be empty")

	t.Logf("PushConfigFile success, JobInstanceID: %s, StepInstanceID: %s", resp.JobInstanceID, resp.StepInstanceID)
}

// TestGetJobInstanceStatus 测试查询作业执行状态接口
func TestGetJobInstanceStatus(t *testing.T) {
	jobService := getJobService(t)

	ctx := context.Background()

	// 构造测试请求
	req := &GetJobInstanceStatusReq{
		BkScopeType:    ScopeTypeBiz,
		BkScopeID:      "100148",
		JobInstanceID:  100,
		ReturnIPResult: false,
	}

	// 执行测试
	resp, err := jobService.GetJobInstanceStatus(ctx, req)

	// 断言
	require.NoError(t, err, "GetJobInstanceStatus should not return error")
	require.NotNil(t, resp, "Response should not be nil")
	require.NotNil(t, resp.JobInstance, "JobInstance should not be nil")
	require.Equal(t, uint64(100), resp.JobInstance.JobInstanceID, "JobInstanceID should match")
	require.Equal(t, "100148", resp.JobInstance.BkScopeID, "BkScopeID should match")
	require.Equal(t, string(ScopeTypeBiz), resp.JobInstance.BkScopeType, "BkScopeType should match")

	// 当 ReturnIPResult 为 true 时，应该返回步骤实例列表
	if req.ReturnIPResult {
		require.NotNil(t, resp.StepInstanceList, "StepInstanceList should not be nil when ReturnIPResult is true")
		if len(resp.StepInstanceList) > 0 {
			stepInstance := resp.StepInstanceList[0]
			require.NotEmpty(t, stepInstance.Name, "StepInstance name should not be empty")
			require.Greater(t, stepInstance.StepInstanceID, uint64(0), "StepInstanceID should be greater than 0")
		}
	}

	t.Logf("GetJobInstanceStatus success, Finished: %v, Status: %s", resp.Finished, resp.JobInstance.Status.String())
}
