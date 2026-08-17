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
	"fmt"
	"net/http"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// System 权限中心中注册的接入系统。
type System struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Managers    []string `json:"managers,omitempty"`
	// Clients 允许调用本系统权限接口的蓝鲸应用列表，不在列表内的应用会被 IAM 拒绝
	Clients     []string `json:"clients"`
	CallbackURL string   `json:"callback_url,omitempty"`
}

// CreateSystemReq 注册系统的请求体。
type CreateSystemReq struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Managers    []string `json:"managers,omitempty"`
	Clients     []string `json:"clients"`
	CallbackURL string   `json:"callback_url,omitempty"`
}

// UpdateSystemReq 更新系统的请求体。
type UpdateSystemReq struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Managers    []string `json:"managers,omitempty"`
	Clients     []string `json:"clients,omitempty"`
	CallbackURL string   `json:"callback_url,omitempty"`
}

// CreateSystem 注册系统，返回系统 ID。
func (c *Client) CreateSystem(kt *kit.Kit, req *CreateSystemReq) (string, error) {
	resp := new(struct {
		ID string `json:"id"`
	})

	err := c.do(kt, &request{
		method: http.MethodPost,
		path:   "/api/v1/open/rbac/model/systems/",
		body:   req,
		result: resp,
	})
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

// GetSystem 查询系统详情，系统不存在时返回的错误可用 IsNotFound 判定。
func (c *Client) GetSystem(kt *kit.Kit) (*System, error) {
	resp := new(System)

	err := c.do(kt, &request{
		method: http.MethodGet,
		path:   fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/", c.cfg.SystemID),
		result: resp,
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// UpdateSystem 更新系统信息。
func (c *Client) UpdateSystem(kt *kit.Kit, req *UpdateSystemReq) error {
	return c.do(kt, &request{
		method: http.MethodPut,
		path:   fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/", c.cfg.SystemID),
		body:   req,
	})
}

// GetSystemAuthToken 获取系统 Auth Token。权限中心回调资源实例接口时，
// 会以 Authorization: Basic base64(bk_iam:{token}) 的形式携带它做认证。
func (c *Client) GetSystemAuthToken(kt *kit.Kit) (string, error) {
	resp := new(struct {
		AuthToken string `json:"auth_token"`
	})

	err := c.do(kt, &request{
		method: http.MethodGet,
		path:   fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/auth-token/", c.cfg.SystemID),
		result: resp,
	})
	if err != nil {
		return "", err
	}

	return resp.AuthToken, nil
}
