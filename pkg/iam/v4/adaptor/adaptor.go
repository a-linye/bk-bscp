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

// Package adaptor maps BSCP's own permission model onto IAM V4 actions and resources.
package adaptor

import (
	"fmt"
	"strconv"

	"github.com/TencentBlueKing/bk-bscp/pkg/iam/meta"
	"github.com/TencentBlueKing/bk-bscp/pkg/iam/v4/client"
	"github.com/TencentBlueKing/bk-bscp/pkg/iam/v4/model"
)

// Mapped 是 meta 域模型映射到 IAM V4 之后的结果。
type Mapped struct {
	// Skip 为 true 表示该资源在 BSCP 侧不做权限控制，直接放行，不需要请求权限中心。
	Skip bool
	// ActionID IAM V4 的操作 ID
	ActionID string
	// ResourceType 授权维度，取 model.ResourceTypeBiz 或 model.ResourceTypeApp；
	// 为空表示该操作与资源无关
	ResourceType string
	// ResourceID 资源实例 ID
	ResourceID string
	// BizID 所属业务 ID。app 维度用它生成拓扑路径，让按业务的授权能覆盖到具体服务
	BizID string
}

// skipped 构造一个直接放行的映射结果。
func skipped() *Mapped {
	return &Mapped{Skip: true}
}

// AuthResource 构建 IAM V4 鉴权接口用的资源。
// auth-by-resources 接口的请求体形如：
//
//	{
//	  "subject": {"type": "user", "id": "admin"},
//	  "action_id": "app_view",
//	  "resources": [
//	    {"id": "101", "attributes": {"_bk_iam_path_": "/biz,2/"}}
//	  ]
//	}
//
// attributes 里的 _bk_iam_path_ 声明资源的归属拓扑。app 维度必须带上，
// 否则按业务授权的用户会被判为无权限——权限中心不会反查拓扑。
// 无关资源类型的操作返回 nil，由调用方省略 resource 字段。
func (m *Mapped) AuthResource() *client.AuthResource {
	if m.ResourceType == "" {
		return nil
	}

	if m.ResourceType == model.ResourceTypeBiz {
		// biz 是拓扑根节点，没有祖先，不需要 attributes。
		res := client.NewAuthResource(m.ResourceID, "")

		return &res
	}

	res := client.NewAuthResource(m.ResourceID,
		client.BuildIAMPath(model.ResourceTypeBiz, m.BizID))

	return &res
}

// ApplyResource 返回权限申请链接用的资源。
// 与鉴权不同，这里的拓扑用 ancestors 数组表达，而不是 attributes 里的路径字符串。
func (m *Mapped) ApplyResource() []client.ApplyResource {
	if m.ResourceType == "" {
		return nil
	}

	if m.ResourceType == model.ResourceTypeBiz {
		return []client.ApplyResource{{ID: m.ResourceID, Type: model.ResourceTypeBiz}}
	}

	return []client.ApplyResource{{
		ID:        m.ResourceID,
		Type:      model.ResourceTypeApp,
		Ancestors: []client.ResourceRef{{Type: model.ResourceTypeBiz, ID: m.BizID}},
	}}
}

// Adapt 把 meta 包中 BSCP 自定义的资源类型与操作，映射为 IAM V4 注册的操作与资源。
func Adapt(a *meta.ResourceAttribute) (*Mapped, error) {
	if a == nil {
		return nil, fmt.Errorf("resource attribute is not set")
	}

	// 上层显式要求跳过时不再看资源类型
	if a.Action == meta.SkipAction {
		return skipped(), nil
	}

	switch a.Type {
	case meta.Biz:
		return adaptBiz(a)
	case meta.App:
		return adaptApp(a)
	case meta.Credential:
		return adaptCredential(a)
	case meta.Audit:
		return adaptAudit(a)
	case meta.ProcConfigMgmt:
		return adaptProcConfigMgmt(a)
	case meta.Commit, meta.ConfigItem, meta.Content, meta.CRInstance, meta.Release,
		meta.ReleasedCI, meta.Strategy, meta.StrategySet, meta.PSH, meta.Repo, meta.Sidecar:
		return skipped(), nil
	default:
		return nil, fmt.Errorf("unsupported bscp auth type: %s", a.Type)
	}
}

func adaptBiz(a *meta.ResourceAttribute) (*Mapped, error) {
	if a.Action != meta.FindBusinessResource {
		return nil, fmt.Errorf("unsupported bscp action for biz: %s", a.Action)
	}

	return bizScoped(model.ActionFindBusinessResource, a.BizID), nil
}

func adaptApp(a *meta.ResourceAttribute) (*Mapped, error) {
	switch a.Action {
	case meta.Create:
		// 服务创建按业务授权，语义是"允许在哪个业务下创建服务"，此时还没有 app 实例。
		return bizScoped(model.ActionAppCreate, a.BizID), nil
	case meta.Find:
		// 查询服务列表属于业务级别操作
		return bizScoped(model.ActionFindBusinessResource, a.BizID), nil
	case meta.View:
		return appScoped(model.ActionAppView, a), nil
	case meta.Update:
		return appScoped(model.ActionAppEdit, a), nil
	case meta.Delete:
		return appScoped(model.ActionAppDelete, a), nil
	case meta.GenerateRelease:
		return appScoped(model.ActionReleaseGenerate, a), nil
	case meta.Publish:
		return appScoped(model.ActionReleasePublish, a), nil
	default:
		return nil, fmt.Errorf("unsupported bscp action for app: %s", a.Action)
	}
}

func adaptCredential(a *meta.ResourceAttribute) (*Mapped, error) {
	switch a.Action {
	case meta.View:
		return bizScoped(model.ActionAppCredentialView, a.BizID), nil
	case meta.Manage:
		return bizScoped(model.ActionAppCredentialManage, a.BizID), nil
	default:
		return nil, fmt.Errorf("unsupported bscp action for credential: %s", a.Action)
	}
}

func adaptAudit(a *meta.ResourceAttribute) (*Mapped, error) {
	if a.Action != meta.View {
		return nil, fmt.Errorf("unsupported bscp action for audit: %s", a.Action)
	}

	return bizScoped(model.ActionAuditView, a.BizID), nil
}

func adaptProcConfigMgmt(a *meta.ResourceAttribute) (*Mapped, error) {
	switch a.Action {
	case meta.View:
		return bizScoped(model.ActionProcConfigMgmtView, a.BizID), nil
	case meta.ProcessOperate:
		return bizScoped(model.ActionProcessOperate, a.BizID), nil
	case meta.Create:
		return bizScoped(model.ActionConfigTemplateCreate, a.BizID), nil
	case meta.Update:
		return bizScoped(model.ActionConfigTemplateEdit, a.BizID), nil
	case meta.Delete:
		return bizScoped(model.ActionConfigTemplateDelete, a.BizID), nil
	case meta.GenerateConfig:
		return bizScoped(model.ActionConfigGenerate, a.BizID), nil
	case meta.ReleaseConfig:
		return bizScoped(model.ActionConfigRelease, a.BizID), nil
	default:
		return nil, fmt.Errorf("unsupported bscp action for process config management: %s", a.Action)
	}
}

// bizScoped 构造按业务维度鉴权的映射。
func bizScoped(actionID string, bizID uint32) *Mapped {
	id := formatID(bizID)

	return &Mapped{
		ActionID:     actionID,
		ResourceType: model.ResourceTypeBiz,
		ResourceID:   id,
		BizID:        id,
	}
}

// appScoped 构造按服务维度鉴权的映射。
func appScoped(actionID string, a *meta.ResourceAttribute) *Mapped {
	return &Mapped{
		ActionID:     actionID,
		ResourceType: model.ResourceTypeApp,
		ResourceID:   formatID(a.ResourceID),
		BizID:        formatID(a.BizID),
	}
}

func formatID(id uint32) string {
	return strconv.FormatUint(uint64(id), 10)
}
