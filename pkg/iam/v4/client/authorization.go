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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// MaxAuthorizationExpireDuration 授权有效期上限，超过该值网关会拒绝。
const MaxAuthorizationExpireDuration = 365 * 24 * time.Hour

// AnyResourceID 资源 ID 取该值时表示对该资源类型的所有实例授权。
const AnyResourceID = "*"

// ResourceRef 授权管理与权限申请接口中的资源引用。
// 注意与鉴权接口的 AuthResource 不同：这里有 type 字段、没有 attributes。
type ResourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Authorization 一条角色授权。IAM V4 的授权单位是角色而不是操作。
// RelatedResourceTypeID 为空表示对无关资源类型的操作授权，此时 Resources 为空数组。
// Resources 中只能包含关联资源类型本身及其祖先资源类型的实例。
// ExpiredAt 是 unix 时间戳，撤销授权时该字段被忽略。
type Authorization struct {
	Subject               Subject       `json:"subject"`
	RoleID                string        `json:"role_id"`
	RelatedResourceTypeID string        `json:"related_resource_type_id"`
	Resources             []ResourceRef `json:"resources"`
	ExpiredAt             int64         `json:"expired_at,omitempty"`
}

// AuthorizationSubject 已被授权的主体及其过期时间。
type AuthorizationSubject struct {
	Subject   Subject `json:"subject"`
	ExpiredAt int64   `json:"expired_at"`
}

// ListAuthorizationSubjectReq 查询授权主体的请求体。
// RelatedResourceTypeID 与 Resource 必须同时提供或同时为空，后者用于查询无关资源类型角色的授权。
type ListAuthorizationSubjectReq struct {
	RoleID                string       `json:"role_id"`
	RelatedResourceTypeID string       `json:"related_resource_type_id,omitempty"`
	Resource              *ResourceRef `json:"resource,omitempty"`
	Page                  int          `json:"page,omitempty"`
	PageSize              int          `json:"page_size,omitempty"`
}

// AddAuthorizations 批量授予角色权限，operator 为操作人，单次最多 MaxAuthorizationBatchSize 条。
func (c *Client) AddAuthorizations(kt *kit.Kit, operator string, items []Authorization) error {
	if err := validateAuthorizationBatch(operator, items); err != nil {
		return err
	}

	return c.do(kt, &request{
		method:   http.MethodPost,
		path:     fmt.Sprintf("/api/v1/open/rbac/mgmt/systems/%s/authorizations/", c.cfg.SystemID),
		body:     items,
		operator: operator,
	})
}

// RevokeAuthorizations 批量撤销角色授权，单次最多 MaxAuthorizationBatchSize 条。
// 该接口是带 body 的 DELETE。
func (c *Client) RevokeAuthorizations(kt *kit.Kit, operator string, items []Authorization) error {
	if err := validateAuthorizationBatch(operator, items); err != nil {
		return err
	}

	// 撤销接口不接受 expired_at，逐条清零以确保它被 omitempty 省略。
	revoking := make([]Authorization, len(items))
	for i, item := range items {
		item.ExpiredAt = 0
		revoking[i] = item
	}

	return c.do(kt, &request{
		method:   http.MethodDelete,
		path:     fmt.Sprintf("/api/v1/open/rbac/mgmt/systems/%s/authorizations/", c.cfg.SystemID),
		body:     revoking,
		operator: operator,
	})
}

// ListAuthorizationSubjects 分页查询某个角色在指定资源上的已授权主体。
func (c *Client) ListAuthorizationSubjects(kt *kit.Kit, req *ListAuthorizationSubjectReq) (
	[]AuthorizationSubject, int, error) {

	if req.Page <= 0 {
		req.Page = 1
	}

	if req.PageSize <= 0 || req.PageSize > MaxPageSize {
		req.PageSize = MaxPageSize
	}

	resp := new(struct {
		Count   int                    `json:"count"`
		Results []AuthorizationSubject `json:"results"`
	})

	err := c.do(kt, &request{
		method: http.MethodPost,
		path: fmt.Sprintf("/api/v1/open/rbac/mgmt/systems/%s/authorizations/query-subject/",
			c.cfg.SystemID),
		body:   req,
		result: resp,
	})
	if err != nil {
		return nil, 0, err
	}

	return resp.Results, resp.Count, nil
}

func validateAuthorizationBatch(operator string, items []Authorization) error {
	if operator == "" {
		return errors.New("operator is required")
	}

	if len(items) == 0 {
		return errors.New("no authorization item given")
	}

	if len(items) > MaxAuthorizationBatchSize {
		return fmt.Errorf("authorization count %d exceeds the limit %d",
			len(items), MaxAuthorizationBatchSize)
	}

	return nil
}
