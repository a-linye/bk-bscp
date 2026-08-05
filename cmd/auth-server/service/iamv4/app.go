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

package iamv4

import (
	"errors"
	"fmt"

	"github.com/TencentBlueKing/bk-bscp/pkg/iam/v4/model"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	pbbase "github.com/TencentBlueKing/bk-bscp/pkg/protocol/core/base"
	pbds "github.com/TencentBlueKing/bk-bscp/pkg/protocol/data-service"
)

// listAppInstances 返回某个业务下的服务实例列表。
// data-service 的 ListInstances 不支持关键字过滤，而 V4 协议要求必须支持对
// display_name 的包含式搜索。这里的做法是带关键字时先拉取该业务下的全部服务再内存过滤，
// 代价可控：单业务的服务数量远小于 MaxPageSize，且只在用户输入搜索词时才走这条路径。
// 若将来单业务服务数逼近上限，应把关键字下推到 data-service 的查询条件里。
func (i *IAM) listAppInstances(kt *kit.Kit, ft Filter, p Page) (*ListInstanceResult, error) {
	if ft.Parent == nil || ft.Parent.ID == "" {
		return nil, errors.New("listing app instances requires a biz parent")
	}

	if ft.Parent.Type != model.ResourceTypeBiz {
		return nil, fmt.Errorf("unexpected parent type for app: %s", ft.Parent.Type)
	}

	if ft.Keyword == "" {
		return i.listAppsByPage(kt, ft.Parent.ID, p)
	}

	return i.listAppsByKeyword(kt, ft.Parent.ID, ft.Keyword, p)
}

// listAppsByPage 无关键字时把分页直接下推给 data-service。
func (i *IAM) listAppsByPage(kt *kit.Kit, bizID string, p Page) (*ListInstanceResult, error) {
	p = p.normalize()

	resp, err := i.apps.ListInstances(kt.RpcCtx(), &pbds.ListInstancesReq{
		ResourceType: model.ResourceTypeApp,
		ParentType:   model.ResourceTypeBiz,
		ParentId:     bizID,
		Page:         &pbbase.BasePage{Start: uint32(p.offset()), Limit: uint32(p.PageSize)},
	})
	if err != nil {
		return nil, err
	}

	results := make([]InstanceBrief, 0, len(resp.Details))
	for _, one := range resp.Details {
		results = append(results, InstanceBrief{ID: one.Id, DisplayName: one.Name})
	}

	return &ListInstanceResult{Count: int(resp.Count), Results: results}, nil
}

// listAppsByKeyword 带关键字时拉全量后内存过滤，再按页返回。
func (i *IAM) listAppsByKeyword(kt *kit.Kit, bizID, keyword string, p Page) (
	*ListInstanceResult, error) {

	resp, err := i.apps.ListInstances(kt.RpcCtx(), &pbds.ListInstancesReq{
		ResourceType: model.ResourceTypeApp,
		ParentType:   model.ResourceTypeBiz,
		ParentId:     bizID,
		Page:         &pbbase.BasePage{Start: 0, Limit: MaxPageSize},
	})
	if err != nil {
		return nil, err
	}

	matched := make([]InstanceBrief, 0, len(resp.Details))
	for _, one := range resp.Details {
		if !matchKeyword(keyword, one.Id, one.Name) {
			continue
		}

		matched = append(matched, InstanceBrief{ID: one.Id, DisplayName: one.Name})
	}

	return &ListInstanceResult{Count: len(matched), Results: pageSlice(matched, p)}, nil
}

// fetchAppInstanceInfo 批量返回服务详情。
// app 的 _bk_iam_path_ 声明它所属的业务，权限中心据此在页面上回显资源拓扑。
func (i *IAM) fetchAppInstanceInfo(kt *kit.Kit, ids []string, selector attrSelector) (
	[]instanceInfo, error) {

	resp, err := i.apps.FetchInstanceInfo(kt.RpcCtx(), &pbds.FetchInstanceInfoReq{
		ResourceType: model.ResourceTypeApp,
		Ids:          ids,
	})
	if err != nil {
		return nil, err
	}

	results := make([]instanceInfo, 0, len(resp.Details))
	for _, detail := range resp.Details {
		info := instanceInfo{"id": detail.Id}

		if selector.need(AttrDisplayName) {
			info[AttrDisplayName] = detail.DisplayName
		}

		if selector.need(AttrIAMPath) {
			// V3 的 path 是数组，V4 只接受单个字符串。app 的拓扑只有一层父级，取首项即可。
			info[AttrIAMPath] = firstPath(detail.Path)
		}

		if selector.need(AttrIAMApprovers) {
			approvers := detail.Approver
			if approvers == nil {
				approvers = []string{}
			}
			info[AttrIAMApprovers] = approvers
		}

		results = append(results, info)
	}

	return results, nil
}

// firstPath 取 V3 风格路径数组的首项。为空时返回空串，由权限中心按无拓扑处理。
func firstPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	return paths[0]
}
