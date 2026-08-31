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

import "github.com/TencentBlueKing/bk-bscp/pkg/iam/v4/client"

// ResourceTypes 返回 BSCP 在 IAM V4 中注册的资源类型。
// biz 必须由 BSCP 自注册：IAM V4 的 ancestors 没有 system 维度，祖先链只能引用本系统内注册的
// 资源类型，实例 ID 仍沿用 CMDB 的 bk_biz_id，实例数据由 BSCP 通过资源回调代理提供。
func ResourceTypes() []client.ResourceType {
	return []client.ResourceType{
		{ID: ResourceTypeBiz, Name: "业务"},
		{ID: ResourceTypeApp, Name: "服务", Ancestors: []string{ResourceTypeBiz}},
	}
}

// Actions 返回 BSCP 在 IAM V4 中注册的操作
func Actions() []client.Action {
	return []client.Action{
		{ID: ActionFindBusinessResource, Name: "业务访问", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionAppCreate, Name: "服务创建", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionAppView, Name: "服务查看", ResourceTypeID: ResourceTypeApp},
		{ID: ActionAppEdit, Name: "服务编辑", ResourceTypeID: ResourceTypeApp},
		{ID: ActionAppDelete, Name: "服务删除", ResourceTypeID: ResourceTypeApp},
		{ID: ActionReleaseGenerate, Name: "生成版本", ResourceTypeID: ResourceTypeApp},
		{ID: ActionReleasePublish, Name: "上线版本", ResourceTypeID: ResourceTypeApp},
		{ID: ActionAppCredentialView, Name: "服务密钥查看", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionAppCredentialManage, Name: "服务密钥管理", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionAuditView, Name: "操作记录查看", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionProcConfigMgmtView, Name: "进程配置管理查看", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionProcessOperate, Name: "进程操作", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionConfigTemplateCreate, Name: "配置模板创建", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionConfigTemplateEdit, Name: "配置模板编辑", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionConfigTemplateDelete, Name: "配置模板删除", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionConfigGenerate, Name: "配置生成", ResourceTypeID: ResourceTypeBiz},
		{ID: ActionConfigRelease, Name: "配置下发", ResourceTypeID: ResourceTypeBiz},
	}
}

// Roles 返回 BSCP 在 IAM V4 中注册的角色
// 业务维度角色的全部操作按 biz 授权；服务维度角色的 find_business_resource 按 biz 授权
// 以放行业务访问前置校验，其余操作按 app 授权。
func Roles() []client.Role {
	return []client.Role{
		{
			ID:          RoleBizAccessor,
			Name:        "业务访问",
			Description: "访问业务的基础权限，含分组、脚本、模板与变量的管理",
			Actions:     bizScoped(ActionFindBusinessResource),
		},
		{
			ID:          RoleBizViewer,
			Name:        "业务只读",
			Description: "查看业务下的服务、密钥、审计与进程配置",
			Actions: bizScoped(ActionFindBusinessResource, ActionAppView,
				ActionAppCredentialView, ActionAuditView, ActionProcConfigMgmtView),
		},
		{
			ID:          RoleBizOperator,
			Name:        "业务运维",
			Description: "业务下的全部操作权限",
			Actions:     bizScoped(allActionIDs()...),
		},
		{
			ID:   RoleAppCreator,
			Name: "服务创建者",
			Description: "在业务下创建服务；创建后自动获得该服务的运维权限，" +
				"访问他人创建的服务需另行申请",
			Actions: bizScoped(ActionFindBusinessResource, ActionAppCreate),
		},
		{
			ID:          RoleCredentialManager,
			Name:        "服务密钥管理员",
			Description: "管理业务下的服务密钥",
			Actions: bizScoped(ActionFindBusinessResource, ActionAppCredentialView,
				ActionAppCredentialManage),
		},
		{
			ID:          RoleProcConfigManager,
			Name:        "进程配置管理员",
			Description: "管理业务下的进程配置与配置模板",
			Actions: bizScoped(ActionFindBusinessResource, ActionProcConfigMgmtView,
				ActionProcessOperate, ActionConfigTemplateCreate, ActionConfigTemplateEdit,
				ActionConfigTemplateDelete, ActionConfigGenerate, ActionConfigRelease),
		},
		{
			ID:          RoleAuditViewer,
			Name:        "操作记录查看者",
			Description: "查看业务下的操作记录",
			Actions:     bizScoped(ActionFindBusinessResource, ActionAuditView),
		},
		{
			ID:          RoleAppOperator,
			Name:        "服务运维",
			Description: "管理指定服务的配置与版本",
			Actions: append(bizScoped(ActionFindBusinessResource),
				appScoped(ActionAppView, ActionAppEdit, ActionAppDelete,
					ActionReleaseGenerate, ActionReleasePublish)...),
		},
		{
			ID:          RoleAppPublisher,
			Name:        "服务发布者",
			Description: "生成并上线指定服务的版本，不含配置编辑",
			Actions: append(bizScoped(ActionFindBusinessResource),
				appScoped(ActionAppView, ActionReleaseGenerate, ActionReleasePublish)...),
		},
		{
			ID:          RoleAppViewer,
			Name:        "服务查看者",
			Description: "查看指定服务",
			Actions:     append(bizScoped(ActionFindBusinessResource), appScoped(ActionAppView)...),
		},
	}
}

// DisplayResourceTypes 返回每个角色的默认展示资源层级配置，键为角色 ID
func DisplayResourceTypes() map[string][]client.DisplayResourceType {
	// 只有 biz 一个授权维度的角色。
	bizScopeOnly := []client.DisplayResourceType{
		{RelatedResourceTypeID: ResourceTypeBiz, DisplayResourceTypeID: ResourceTypeBiz},
	}
	// 同时有 biz 与 app 两个授权维度的角色，两者都只能展示到 biz。
	bothScopes := []client.DisplayResourceType{
		{RelatedResourceTypeID: ResourceTypeBiz, DisplayResourceTypeID: ResourceTypeBiz},
		{RelatedResourceTypeID: ResourceTypeApp, DisplayResourceTypeID: ResourceTypeBiz},
	}

	return map[string][]client.DisplayResourceType{
		RoleBizAccessor:       bizScopeOnly,
		RoleBizViewer:         bizScopeOnly,
		RoleBizOperator:       bizScopeOnly,
		RoleAppCreator:        bizScopeOnly,
		RoleCredentialManager: bizScopeOnly,
		RoleProcConfigManager: bizScopeOnly,
		RoleAuditViewer:       bizScopeOnly,
		RoleAppOperator:       bothScopes,
		RoleAppPublisher:      bothScopes,
		RoleAppViewer:         bothScopes,
	}
}

// bizScoped 把一组操作全部声明为按业务维度授权。
func bizScoped(actionIDs ...string) []client.RoleAction {
	return scoped(ResourceTypeBiz, actionIDs)
}

// appScoped 把一组操作全部声明为按服务维度授权。
func appScoped(actionIDs ...string) []client.RoleAction {
	return scoped(ResourceTypeApp, actionIDs)
}

func scoped(resourceTypeID string, actionIDs []string) []client.RoleAction {
	actions := make([]client.RoleAction, 0, len(actionIDs))
	for _, id := range actionIDs {
		actions = append(actions, client.RoleAction{ID: id, ResourceTypeID: resourceTypeID})
	}

	return actions
}

func allActionIDs() []string {
	actions := Actions()
	ids := make([]string, 0, len(actions))
	for _, a := range actions {
		ids = append(ids, a.ID)
	}

	return ids
}
