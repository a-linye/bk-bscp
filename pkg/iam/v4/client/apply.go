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

package client

import (
	"net/http"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// ApplyResource 权限申请链接中的资源。
// Ancestors 需给出从根资源到该实例上一级的完整路径，app 类型即 [{type: biz, id: bk_biz_id}]。
type ApplyResource struct {
	ID        string        `json:"id"`
	Type      string        `json:"type"`
	Ancestors []ResourceRef `json:"ancestors,omitempty"`
}

// ApplyPermission 权限申请链接中的一条权限。
type ApplyPermission struct {
	ActionID  string          `json:"action_id"`
	Resources []ApplyResource `json:"resources,omitempty"`
}

// GeneratePermApplyURL 生成无权限时的申请链接。
// 该接口的路径不含 system_id，系统 ID 在请求体中给出。
func (c *Client) GeneratePermApplyURL(kt *kit.Kit, permissions []ApplyPermission) (string, error) {
	body := struct {
		SystemID    string            `json:"system_id"`
		Permissions []ApplyPermission `json:"permissions"`
	}{SystemID: c.cfg.SystemID, Permissions: permissions}

	resp := new(struct {
		URL string `json:"url"`
	})

	err := c.do(kt, &request{
		method: http.MethodPost,
		path:   "/api/v1/open/application/permission-apply-urls/",
		body:   body,
		result: resp,
	})
	if err != nil {
		return "", err
	}

	return resp.URL, nil
}
