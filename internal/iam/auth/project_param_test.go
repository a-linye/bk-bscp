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

package auth

import (
	"strings"
	"testing"

	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

func TestParseProjectParam(t *testing.T) {
	cases := []struct {
		name       string
		seg        string
		wantID     uint32
		wantKey    string
		wantErr    bool
		errKeyword string
	}{
		{name: "数字 ID（控制台调用）", seg: "42", wantID: 42},
		{name: "数字 ID 为 0 时交由下游报不存在", seg: "0", wantID: 0},
		{name: "项目 Key", seg: "BK-BSCP-00042", wantKey: "BK-BSCP-00042"},
		{name: "项目 Key 的 ID 超过 5 位", seg: "BK-BSCP-123456", wantKey: "BK-BSCP-123456"},
		{name: "Key 位数不足", seg: "BK-BSCP-42", wantErr: true, errKeyword: "BK-BSCP-00042"},
		{name: "Key 前缀错误", seg: "BKBSCP-00042", wantErr: true, errKeyword: "BK-BSCP-00042"},
		{name: "Key 后缀非数字", seg: "BK-BSCP-abcde", wantErr: true, errKeyword: "BK-BSCP-00042"},
		{name: "空段", seg: "", wantErr: true},
		{name: "超出 uint32 的数字不当作 ID，也不是合法 Key", seg: "4294967296", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, key, err := parseProjectParam(c.seg)

			if c.wantErr {
				if err == nil {
					t.Fatalf("parseProjectParam(%q) 期望报错，实际返回 id=%d key=%q", c.seg, id, key)
				}
				if c.errKeyword != "" && !strings.Contains(err.Error(), c.errKeyword) {
					t.Errorf("错误信息应包含 %q 以提示正确格式，实际为 %q", c.errKeyword, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("parseProjectParam(%q) 意外报错: %v", c.seg, err)
			}
			if id != c.wantID {
				t.Errorf("projectID = %d, 期望 %d", id, c.wantID)
			}
			if key != c.wantKey {
				t.Errorf("projectKey = %q, 期望 %q", key, c.wantKey)
			}
		})
	}
}

// TestParseProjectParamAcceptsGeneratedKey 保证解析逻辑与 Key 的生成逻辑同源，
// 避免 GenerateProjectKey 改动后中间件静默拒绝合法 Key。
func TestParseProjectParamAcceptsGeneratedKey(t *testing.T) {
	for _, id := range []uint32{1, 42, 99999, 100000, 4294967295} {
		generated := table.GenerateProjectKey(id)

		gotID, gotKey, err := parseProjectParam(generated)
		if err != nil {
			t.Errorf("GenerateProjectKey(%d) = %q 应被接受，却报错: %v", id, generated, err)
			continue
		}
		if gotKey != generated {
			t.Errorf("GenerateProjectKey(%d) = %q 应按 Key 解析，实际 id=%d key=%q", id, generated, gotID, gotKey)
		}
	}
}