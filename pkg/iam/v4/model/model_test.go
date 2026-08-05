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

package model

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// idPattern 是 IAM V4 对资源类型、操作与角色 ID 的格式要求。
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}[a-z0-9]$`)

// validScopes 是每个资源类型的合法授权维度集合：自身加上全部祖先。
var validScopes = map[string]map[string]bool{
	ResourceTypeBiz: {ResourceTypeBiz: true},
	ResourceTypeApp: {ResourceTypeApp: true, ResourceTypeBiz: true},
}

func TestIDsMatchIAMPattern(t *testing.T) {
	for _, rt := range ResourceTypes() {
		require.Regexp(t, idPattern, rt.ID)
		require.LessOrEqual(t, len(rt.ID), 32)
	}

	for _, a := range Actions() {
		require.Regexp(t, idPattern, a.ID)
		require.LessOrEqual(t, len(a.ID), 32)
	}

	for _, r := range Roles() {
		require.Regexp(t, idPattern, r.ID)
		require.LessOrEqual(t, len(r.ID), 32)
	}
}

func TestModelCounts(t *testing.T) {
	require.Len(t, ResourceTypes(), 2)
	require.Len(t, Actions(), 17)
	require.Len(t, Roles(), 9)
	require.Len(t, DisplayResourceTypes(), 9)
}

func TestIDsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, a := range Actions() {
		require.False(t, seen[a.ID], "duplicated action id %s", a.ID)
		seen[a.ID] = true
	}

	seen = make(map[string]bool)
	for _, r := range Roles() {
		require.False(t, seen[r.ID], "duplicated role id %s", r.ID)
		seen[r.ID] = true
	}
}

// 每个操作关联的资源类型必须已注册。
func TestActionResourceTypesAreRegistered(t *testing.T) {
	registered := make(map[string]bool)
	for _, rt := range ResourceTypes() {
		registered[rt.ID] = true
	}

	for _, a := range Actions() {
		require.True(t, registered[a.ResourceTypeID],
			"action %s references unregistered resource type %s", a.ID, a.ResourceTypeID)
	}
}

// 角色中引用的操作必须都已定义，且其授权维度必须是该操作关联的资源类型本身或其祖先。
func TestRoleActionsAreValid(t *testing.T) {
	actionScope := make(map[string]string)
	for _, a := range Actions() {
		actionScope[a.ID] = a.ResourceTypeID
	}

	for _, role := range Roles() {
		require.NotEmpty(t, role.Actions, "role %s has no action", role.ID)

		for _, ra := range role.Actions {
			ownType, ok := actionScope[ra.ID]
			require.True(t, ok, "role %s references undefined action %s", role.ID, ra.ID)
			require.True(t, validScopes[ownType][ra.ResourceTypeID],
				"role %s action %s has invalid scope %s", role.ID, ra.ID, ra.ResourceTypeID)
		}
	}
}

// 每个角色的展示层级配置必须覆盖它实际用到的全部授权维度，
// 且展示粒度必须是授权粒度自身或其祖先。
func TestDisplayResourceTypesCoverRoleScopes(t *testing.T) {
	display := DisplayResourceTypes()

	for _, role := range Roles() {
		configured := make(map[string]string)
		for _, d := range display[role.ID] {
			configured[d.RelatedResourceTypeID] = d.DisplayResourceTypeID
		}

		for _, ra := range role.Actions {
			displayType, ok := configured[ra.ResourceTypeID]
			require.True(t, ok, "role %s scope %s has no display config", role.ID, ra.ResourceTypeID)
			require.True(t, validScopes[ra.ResourceTypeID][displayType],
				"role %s display %s is not an ancestor of scope %s",
				role.ID, displayType, ra.ResourceTypeID)
		}
	}
}

// 所有业务接口都有 find_business_resource 前置校验，任何角色缺了它都会被第一道校验拦住。
func TestEveryRoleGrantsBusinessAccess(t *testing.T) {
	for _, role := range Roles() {
		var found bool
		for _, ra := range role.Actions {
			if ra.ID == ActionFindBusinessResource {
				found = true
				require.Equal(t, ResourceTypeBiz, ra.ResourceTypeID)

				break
			}
		}
		require.True(t, found, "role %s does not grant %s", role.ID, ActionFindBusinessResource)
	}
}

// biz_operator 覆盖全部 17 个操作，是 V3 常用权限「业务运维」的等价物。
func TestBizOperatorCoversAllActions(t *testing.T) {
	for _, role := range Roles() {
		if role.ID != RoleBizOperator {
			continue
		}

		require.Len(t, role.Actions, len(Actions()))

		return
	}

	t.Fatalf("role %s not found", RoleBizOperator)
}

// app_creator 是唯一"能创建服务但不能读写他人服务"的角色，它的存在让创建者自动授权
// 产生真实的权限增量。三个断言各自锁住一条设计决策，改动前请先读注释。
func TestAppCreatorScopesCreationOnly(t *testing.T) {
	var actions map[string]string
	for _, role := range Roles() {
		if role.ID != RoleAppCreator {
			continue
		}

		actions = make(map[string]string, len(role.Actions))
		for _, ra := range role.Actions {
			actions[ra.ID] = ra.ResourceTypeID
		}
	}
	require.NotNil(t, actions, "role %s not found", RoleAppCreator)

	// 一、能建服务，且必须按 biz 授权——app_create 的语义是"允许在哪个业务下创建"。
	require.Equal(t, ResourceTypeBiz, actions[ActionAppCreate],
		"app_create must be granted per business")

	// 二、放行业务访问前置校验，否则持有者连业务都进不去。
	require.Equal(t, ResourceTypeBiz, actions[ActionFindBusinessResource])

	// 三、刻意不含 app_view。这不是遗漏：app_view 在纯 biz 维度角色里只能按 biz 授权，
	// 一给就放开了业务下所有服务的详情访问，与本角色"他人服务需另行申请"的语义冲突。
	// 自己创建的服务由创建者自动授权（app_operator on 该 app）覆盖。
	require.NotContains(t, actions, ActionAppView,
		"app_view would leak read access to every app under the business")

	require.Len(t, actions, 2, "keep this role minimal")
}

// 服务维度角色必须同时含 biz 与 app 两种授权维度，这是 IAM V4 允许角色跨维度的直接体现，
// 也是 dev 环境需要实测确认的关键假设。
func TestAppScopedRolesAreCrossDimension(t *testing.T) {
	appRoles := map[string]bool{
		RoleAppOperator: true, RoleAppPublisher: true, RoleAppViewer: true,
	}

	for _, role := range Roles() {
		if !appRoles[role.ID] {
			continue
		}

		scopes := make(map[string]bool)
		for _, ra := range role.Actions {
			scopes[ra.ResourceTypeID] = true
		}
		require.True(t, scopes[ResourceTypeBiz] && scopes[ResourceTypeApp],
			"role %s should span both biz and app scopes, got %v", role.ID, scopes)
	}
}
