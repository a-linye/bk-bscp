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

import "testing"

// TestBizsOfTmplSpace 验证默认模板空间缓存按 业务+项目 组合维度隔离，
// 避免同业务下新项目因 biz 级缓存命中而跳过默认空间创建。
func TestBizsOfTmplSpace(t *testing.T) {
	c := BizsOfTmplSpace{pairs: make(map[bizProject]struct{})}

	if c.Has(1, 100) {
		t.Fatal("empty cache should not hit")
	}

	c.Set(1, 100)

	if !c.Has(1, 100) {
		t.Fatal("set pair should hit")
	}
	if c.Has(1, 101) {
		t.Fatal("same biz with another project should not hit")
	}
	if c.Has(2, 100) {
		t.Fatal("another biz with same project should not hit")
	}
}
