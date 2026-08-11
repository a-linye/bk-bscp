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

package service

import (
	"reflect"
	"strings"
	"testing"

	_ "github.com/TencentBlueKing/bk-bscp/internal/i18n/translations"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/i18n"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

func TestDedupConfigTemplateIDs(t *testing.T) {
	cases := []struct {
		name string
		ids  []uint32
		want []uint32
	}{
		{name: "去重并保持顺序", ids: []uint32{34, 12, 34, 12, 56}, want: []uint32{34, 12, 56}},
		{name: "剔除0值", ids: []uint32{0, 12, 0}, want: []uint32{12}},
		{name: "空输入", ids: nil, want: []uint32{}},
		{name: "全为0", ids: []uint32{0, 0}, want: []uint32{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dedupConfigTemplateIDs(c.ids); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("dedupConfigTemplateIDs(%v) = %v, want %v", c.ids, got, c.want)
			}
		})
	}
}

func TestMissingConfigTemplateIDs(t *testing.T) {
	templates := func(ids ...uint32) []*table.ConfigTemplate {
		result := make([]*table.ConfigTemplate, 0, len(ids))
		for _, id := range ids {
			result = append(result, &table.ConfigTemplate{ID: id})
		}
		return result
	}

	cases := []struct {
		name      string
		requested []uint32
		found     []*table.ConfigTemplate
		want      []uint32
	}{
		{
			name:      "全部命中",
			requested: []uint32{12, 34},
			found:     templates(34, 12),
			want:      []uint32{},
		},
		{
			name:      "缺失项全部列出且保持请求顺序",
			requested: []uint32{12, 34, 56},
			found:     templates(34),
			want:      []uint32{12, 56},
		},
		{
			name:      "全部缺失",
			requested: []uint32{12, 34},
			found:     nil,
			want:      []uint32{12, 34},
		},
		{
			name:      "跳过空记录",
			requested: []uint32{12},
			found:     []*table.ConfigTemplate{nil},
			want:      []uint32{12},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := missingConfigTemplateIDs(c.requested, c.found); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("missingConfigTemplateIDs(%v) = %v, want %v", c.requested, got, c.want)
			}
		})
	}
}

func TestJoinConfigTemplateIDs(t *testing.T) {
	cases := []struct {
		name string
		ids  []uint32
		want string
	}{
		{name: "单个", ids: []uint32{817}, want: "817"},
		{name: "多个", ids: []uint32{817, 999817}, want: "817, 999817"},
		{name: "空输入", ids: nil, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := joinConfigTemplateIDs(c.ids); got != c.want {
				t.Fatalf("joinConfigTemplateIDs(%v) = %q, want %q", c.ids, got, c.want)
			}
		})
	}
}

// TestMissingIDsMessageKeepsRawIDs 防止缺失ID提示被 i18n 的数字格式化改写。
// 直接把 []uint32 交给 i18n 时，999817 会按语言习惯渲染成 999,817，看起来像两个ID。
func TestMissingIDsMessageKeepsRawIDs(t *testing.T) {
	idList := joinConfigTemplateIDs([]uint32{999817, 999818})
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			got := i18n.T(&kit.Kit{Lang: lang}, "config templates not found for ids: %s", idList)
			for _, id := range []string{"999817", "999818"} {
				if !strings.Contains(got, id) {
					t.Errorf("提示中缺少原始ID %s: %s", id, got)
				}
			}
			if strings.Contains(got, "999,817") {
				t.Errorf("ID 被加了千分位分隔符: %s", got)
			}
		})
	}
}
