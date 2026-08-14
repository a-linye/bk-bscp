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

func testProcess(ccProcessID uint32) *table.Process {
	return &table.Process{Attachment: &table.ProcessAttachment{CcProcessID: ccProcessID}}
}

func TestFilterBoundCcProcessIDs(t *testing.T) {
	processes := []*table.Process{
		testProcess(11),
		testProcess(22),
		testProcess(11),
		testProcess(33),
		nil,
		{Attachment: nil},
		testProcess(0),
	}

	cases := []struct {
		name     string
		boundIDs []uint32
		want     []uint32
	}{
		{name: "只保留已绑定进程并保持范围内顺序", boundIDs: []uint32{33, 11}, want: []uint32{11, 33}},
		{name: "未绑定进程静默跳过", boundIDs: []uint32{11}, want: []uint32{11}},
		{name: "全部未绑定得到空集", boundIDs: []uint32{99}, want: []uint32{}},
		{name: "绑定集合为空得到空集", boundIDs: nil, want: []uint32{}},
		{name: "剔除绑定集合中的0值", boundIDs: []uint32{0, 22}, want: []uint32{22}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := filterBoundCcProcessIDs(processes, c.boundIDs)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("filterBoundCcProcessIDs() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNoExecutableBindingsMessageDistinctFromNoProcesses(t *testing.T) {
	noProcess := i18n.T(&kit.Kit{Lang: "en"}, "no processes found for biz %d with provided operate range", uint32(1))
	noBinding := i18n.T(&kit.Kit{Lang: "en"}, "no executable config bindings in the operate range")
	if noProcess == noBinding {
		t.Fatalf("无进程与零绑定文案不可相同: %s", noProcess)
	}
	if !strings.Contains(noBinding, "no executable config bindings") {
		t.Fatalf("零绑定文案缺少可识别短语: %s", noBinding)
	}
	if strings.Contains(noBinding, "no processes found") {
		t.Fatalf("零绑定文案不应混用无进程表述: %s", noBinding)
	}

	zhNoProcess := i18n.T(&kit.Kit{Lang: "zh"}, "no processes found for biz %d with provided operate range", uint32(1))
	zhNoBinding := i18n.T(&kit.Kit{Lang: "zh"}, "no executable config bindings in the operate range")
	if zhNoProcess == zhNoBinding {
		t.Fatalf("中文无进程与零绑定文案不可相同: %s", zhNoProcess)
	}
}
