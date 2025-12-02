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
	"fmt"
	"net/url"
	"strconv"
)

var (
	// pushConfigFile 分发配置文件，此接口用于分发配置文件等小的纯文本文件
	pushConfigFile = "%s/api/v3/push_config_file"
	// getJobInstanceStatus 根据作业实例 ID 查询作业执行状态
	getJobInstanceStatus = "%s/api/v3/get_job_instance_status"
)

// PushConfigFile 分发配置文件
func (jobService *Service) PushConfigFile(ctx context.Context, req *PushConfigFileReq) (*PushConfigFileResp, error) {
	url := fmt.Sprintf(pushConfigFile, jobService.host)

	resp := new(PushConfigFileResp)
	if err := jobService.doRequest(ctx, POST, url, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetJobInstanceStatus 根据作业实例 ID 查询作业执行状态
func (jobService *Service) GetJobInstanceStatus(ctx context.Context, req *GetJobInstanceStatusReq) (*GetJobInstanceStatusResp, error) {
	baseURL := fmt.Sprintf(getJobInstanceStatus, jobService.host)
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse url failed: %w", err)
	}

	// 设置 query 参数
	params := url.Values{}
	params.Set("bk_scope_type", string(req.BkScopeType))
	params.Set("bk_scope_id", req.BkScopeID)
	params.Set("job_instance_id", strconv.FormatUint(req.JobInstanceID, 10))
	params.Set("return_ip_result", strconv.FormatBool(req.ReturnIPResult))
	u.RawQuery = params.Encode()

	resp := new(GetJobInstanceStatusResp)
	if err := jobService.doRequest(ctx, GET, u.String(), nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
