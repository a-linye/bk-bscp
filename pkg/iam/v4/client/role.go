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
	"strings"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// RoleAction 角色中的一个操作。ResourceTypeID 是该操作在此角色内的授权维度，
// 取值须为该操作自身关联的资源类型或其祖先；操作为无关资源类型时留空。
// 同一角色内不同操作可以填不同的值，即一个角色可以跨多个资源维度。
type RoleAction struct {
	ID             string `json:"id"`
	ResourceTypeID string `json:"resource_type_id"`
}

// Role 角色，IAM V4 的授权单位。
type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Actions     []RoleAction `json:"actions"`
}

// UpdateRoleReq 更新角色的请求体。
type UpdateRoleReq struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// DisplayResourceType 角色的展示资源层级配置。
// RelatedResourceTypeID 是授权粒度，DisplayResourceTypeID 是用户申请该角色时至多可配置到的层级。
// 后者须为前者自身或其祖先，且同一棵资源拓扑树下的展示层级必须一致。
type DisplayResourceType struct {
	RelatedResourceTypeID string `json:"related_resource_type_id"`
	DisplayResourceTypeID string `json:"display_resource_type_id"`
}

// BatchCreateRoles 批量创建角色，返回创建成功的 ID 列表。请求体顶层为数组。
func (c *Client) BatchCreateRoles(kt *kit.Kit, roles []Role) ([]string, error) {
	var ids []string

	err := c.do(kt, &request{
		method: http.MethodPost,
		path:   fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/roles/", c.cfg.SystemID),
		body:   roles,
		result: &ids,
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// ListRoles 分页查询角色，返回当前页数据与总数。
func (c *Client) ListRoles(kt *kit.Kit, page, pageSize int) ([]Role, int, error) {
	resp := new(struct {
		Count   int    `json:"count"`
		Results []Role `json:"results"`
	})

	err := c.do(kt, &request{
		method: http.MethodGet,
		path:   fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/roles/", c.cfg.SystemID),
		query:  pageQuery(page, pageSize),
		result: resp,
	})
	if err != nil {
		return nil, 0, err
	}

	return resp.Results, resp.Count, nil
}

// UpdateRole 更新角色的名称与描述。
func (c *Client) UpdateRole(kt *kit.Kit, id string, req *UpdateRoleReq) error {
	return c.do(kt, &request{
		method: http.MethodPut,
		path:   fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/roles/%s/", c.cfg.SystemID, id),
		body:   req,
	})
}

// DeleteRole 删除角色。
func (c *Client) DeleteRole(kt *kit.Kit, id string) error {
	return c.do(kt, &request{
		method: http.MethodDelete,
		path:   fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/roles/%s/", c.cfg.SystemID, id),
	})
}

// BatchCreateRoleActions 向角色批量添加操作。
// 角色变更会立即影响已持有该角色的用户与用户组，调整前需评估影响范围。
func (c *Client) BatchCreateRoleActions(kt *kit.Kit, roleID string, actions []RoleAction) error {
	return c.do(kt, &request{
		method: http.MethodPost,
		path: fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/roles/%s/actions/",
			c.cfg.SystemID, roleID),
		body: actions,
	})
}

// BatchDeleteRoleActions 从角色批量移除操作。
// 与添加操作不同，该接口通过 query 参数 ids 传入逗号连接的操作 ID，请求体为空。
// 若移除某个授权维度下的全部操作，该维度下不应存在授权，否则 IAM 会拒绝。
func (c *Client) BatchDeleteRoleActions(kt *kit.Kit, roleID string, actionIDs []string) error {
	if len(actionIDs) == 0 {
		return errors.New("no action id given")
	}

	return c.do(kt, &request{
		method: http.MethodDelete,
		path: fmt.Sprintf("/api/v1/open/rbac/model/systems/%s/roles/%s/actions/",
			c.cfg.SystemID, roleID),
		query: map[string]string{"ids": strings.Join(actionIDs, ",")},
	})
}

// UpdateRoleDisplayResourceTypes 全量覆盖角色的展示资源层级配置，列表不能为空。
// 未传入的关联资源类型会回退为默认值（拓扑树第一层资源类型）后整体校验。
// 注意路径前缀是 /api/v1/open/mgmt/，不含 rbac；成功响应为 204 无响应体。
func (c *Client) UpdateRoleDisplayResourceTypes(kt *kit.Kit, roleID string,
	types []DisplayResourceType) error {

	body := struct {
		DisplayResourceTypes []DisplayResourceType `json:"display_resource_types"`
	}{DisplayResourceTypes: types}

	return c.do(kt, &request{
		method: http.MethodPut,
		path: fmt.Sprintf("/api/v1/open/mgmt/systems/%s/roles/%s/display-resource-types/",
			c.cfg.SystemID, roleID),
		body: body,
	})
}

// ListRoleDisplayResourceTypes 查询角色的完整展示资源层级配置，
// 未显式配置的关联资源类型会以默认值一并返回，无关资源类型不包含在结果中。
// 响应的 data 直接是数组，没有分页包装。
func (c *Client) ListRoleDisplayResourceTypes(kt *kit.Kit, roleID string) (
	[]DisplayResourceType, error) {

	var types []DisplayResourceType

	err := c.do(kt, &request{
		method: http.MethodGet,
		path: fmt.Sprintf("/api/v1/open/mgmt/systems/%s/roles/%s/display-resource-types/",
			c.cfg.SystemID, roleID),
		result: &types,
	})
	if err != nil {
		return nil, err
	}

	return types, nil
}
