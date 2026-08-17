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
	"encoding/json"
	"errors"
	"fmt"

	structpb "google.golang.org/protobuf/types/known/structpb"

	pbas "github.com/TencentBlueKing/bk-bscp/pkg/protocol/auth-server"
)

// ParsePullResourceReq 把 gRPC 请求转换为 V4 的回调请求。
func ParsePullResourceReq(req *pbas.PullResourceReq) (*PullResourceReq, error) {
	if req == nil {
		return nil, errors.New("pull resource request is nil")
	}

	if req.GetType() == "" {
		return nil, errors.New("resource type is required")
	}

	if req.GetMethod() == "" {
		return nil, errors.New("callback method is required")
	}

	parsed := &PullResourceReq{
		Type:     req.GetType(),
		Method:   req.GetMethod(),
		Requires: req.GetRequires(),
		Page: Page{
			Page:     int(req.GetPage().GetPage()),
			PageSize: int(req.GetPage().GetPageSize()),
		},
	}

	if err := decodeStruct(req.GetFilter(), &parsed.Filter); err != nil {
		return nil, fmt.Errorf("decode filter failed, err: %v", err)
	}

	// 个别权限中心版本把 requires 放在 filter 内，顶层取不到时再从 filter 中读取。
	if len(parsed.Requires) == 0 {
		parsed.Requires = extractRequires(req.GetFilter())
	}

	return parsed, nil
}

// MarshalResp 把回调结果包装成 V4 的响应体 {"data": ...}。
func MarshalResp(data any) (*structpb.Struct, error) {
	raw, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		return nil, fmt.Errorf("marshal callback response failed, err: %v", err)
	}

	resp := new(structpb.Struct)
	if err := resp.UnmarshalJSON(raw); err != nil {
		return nil, fmt.Errorf("convert callback response to struct failed, err: %v", err)
	}

	return resp, nil
}

// decodeStruct 把 protobuf 的 Struct 转成目标结构体。nil 时不做任何事，保持零值。
func decodeStruct(src *structpb.Struct, dst any) error {
	if src == nil {
		return nil
	}

	raw, err := src.MarshalJSON()
	if err != nil {
		return err
	}

	return json.Unmarshal(raw, dst)
}

// extractRequires 从 filter 里读 requires 字段，兼容把它放在 filter 内的形态。
func extractRequires(filter *structpb.Struct) []string {
	if filter == nil {
		return nil
	}

	value, ok := filter.GetFields()["requires"]
	if !ok {
		return nil
	}

	list := value.GetListValue()
	if list == nil {
		return nil
	}

	requires := make([]string, 0, len(list.GetValues()))
	for _, item := range list.GetValues() {
		if name := item.GetStringValue(); name != "" {
			requires = append(requires, name)
		}
	}

	return requires
}
