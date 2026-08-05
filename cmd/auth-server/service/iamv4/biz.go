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
	"strings"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// listBizInstances 返回业务实例列表。
//
// V4 下 biz 由 BSCP 自注册，实例数据必须由 BSCP 提供，权限中心不再向 CMDB 获取。
// 这里代理 space.Manager，它已按租户缓存全量业务 10 分钟，
// 因此关键字过滤与分页都在内存完成，不额外请求 CMDB。
func (i *IAM) listBizInstances(kt *kit.Kit, ft Filter, p Page) (*ListInstanceResult, error) {
	// biz 是拓扑根节点，不接受 parent 过滤。请求中携带该参数时直接忽略而非报错，
	// 避免权限中心传入多余参数导致整个下拉框不可用。
	all := i.bizs.AllSpaces(kt.Ctx)

	matched := make([]InstanceBrief, 0, len(all))
	for _, s := range all {
		if !matchKeyword(ft.Keyword, s.SpaceId, s.SpaceName) {
			continue
		}

		matched = append(matched, InstanceBrief{ID: s.SpaceId, DisplayName: s.SpaceName})
	}

	return &ListInstanceResult{Count: len(matched), Results: pageSlice(matched, p)}, nil
}

// fetchBizInstanceInfo 批量返回业务详情。biz 是拓扑根节点，没有 _bk_iam_path_。
func (i *IAM) fetchBizInstanceInfo(kt *kit.Kit, ids []string, selector attrSelector) (
	[]instanceInfo, error) {

	spaces, err := i.bizs.QuerySpace(kt.Ctx, ids)
	if err != nil {
		return nil, err
	}

	nameOf := make(map[string]string, len(spaces))
	for _, s := range spaces {
		nameOf[s.SpaceId] = s.SpaceName
	}

	// 按请求顺序返回，查不到的实例跳过而不报错——业务可能已在 CMDB 侧归档。
	results := make([]instanceInfo, 0, len(ids))
	for _, id := range ids {
		name, ok := nameOf[id]
		if !ok {
			continue
		}

		info := instanceInfo{"id": id}
		if selector.need(AttrDisplayName) {
			info[AttrDisplayName] = name
		}
		if selector.need(AttrIAMApprovers) {
			// 业务的实例审批人需要 CMDB 的 bk_biz_maintainer 字段，
			// space.Manager 当前不返回它，留空表示走系统默认审批流。
			info[AttrIAMApprovers] = []string{}
		}

		results = append(results, info)
	}

	return results, nil
}

// pageSlice 按 V4 的页码分页取子集。
func pageSlice(all []InstanceBrief, p Page) []InstanceBrief {
	p = p.normalize()

	start := p.offset()
	if start >= len(all) {
		return []InstanceBrief{}
	}

	end := start + p.PageSize
	if end > len(all) {
		end = len(all)
	}

	return all[start:end]
}

// contains 做大小写不敏感的包含式匹配，业务与服务名常含中英文混排。
func contains(text, keyword string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(keyword))
}
