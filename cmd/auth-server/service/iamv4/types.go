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

// Package iamv4 implements the BK-IAM V4 resource callback API.
package iamv4

// 回调方法名
const (
	// MethodListInstance 按过滤条件分页查询实例。
	MethodListInstance = "list_instance"
	// MethodFetchInstanceInfo 批量查询实例详情。
	MethodFetchInstanceInfo = "fetch_instance_info"
)

// 实例属性名。
const (
	// AttrDisplayName 展示名称。权限中心用它校验用户提交的资源名称，必须实现。
	AttrDisplayName = "display_name"
	// AttrIAMPath 资源拓扑路径，格式为 /ParentType,ParentInstanceID/。
	AttrIAMPath = "_bk_iam_path_"
	// AttrIAMApprovers 实例审批人。
	AttrIAMApprovers = "_bk_iam_approvers_"
)

// MaxPageSize 是 list_instance 的分页上限，由 IAM 协议规定。
const MaxPageSize = 1000

// PullResourceReq 是权限中心回调的统一请求体。
type PullResourceReq struct {
	// Type 查询的资源类型
	Type string `json:"type"`
	// Method 查询方式，取值见 MethodListInstance / MethodFetchInstanceInfo
	Method string `json:"method"`
	// Filter 按 Method 传入不同的过滤参数
	Filter Filter `json:"filter"`
	// Page list_instance 时必填
	Page Page `json:"page"`
	// Requires 需要返回的属性列表，仅 fetch_instance_info 使用；
	// 注意它在请求体顶层，不在 Filter 内
	Requires []string `json:"requires"`
}

// Filter 是回调请求的过滤条件，字段按 Method 取用。
type Filter struct {
	// Parent 资源的直接上级，list_instance 使用
	Parent *Parent `json:"parent"`
	// Keyword 搜索关键字，list_instance 使用，要求对 display_name 做包含式匹配
	Keyword string `json:"keyword"`
	// IDs 实例 ID 列表，fetch_instance_info 使用
	IDs []string `json:"ids"`
}

// Parent 是资源的直接上级实例。
type Parent struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Page 是 V4 的分页参数，页码从 1 开始。
type Page struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// normalize 把缺省或越界的分页参数收敛到合法范围。
func (p Page) normalize() Page {
	if p.Page <= 0 {
		p.Page = 1
	}

	if p.PageSize <= 0 || p.PageSize > MaxPageSize {
		p.PageSize = MaxPageSize
	}

	return p
}

// offset 返回该页的起始下标。
func (p Page) offset() int {
	return (p.Page - 1) * p.PageSize
}

// ListInstanceResult 是 list_instance 的响应数据。
type ListInstanceResult struct {
	// Count 满足条件的实例总数，不受分页影响
	Count int `json:"count"`
	// Results 当前页的实例列表
	Results []InstanceBrief `json:"results"`
}

// InstanceBrief 是实例的最简表示。
type InstanceBrief struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// instanceInfo 是 fetch_instance_info 的单条响应。
type instanceInfo map[string]any

// attrSelector 判断某个属性是否需要返回。requires 为空表示返回全部属性。
type attrSelector struct {
	all     bool
	require map[string]bool
}

func newAttrSelector(requires []string) attrSelector {
	if len(requires) == 0 {
		return attrSelector{all: true}
	}

	require := make(map[string]bool, len(requires))
	for _, name := range requires {
		require[name] = true
	}

	return attrSelector{require: require}
}

func (s attrSelector) need(attr string) bool {
	return s.all || s.require[attr]
}
