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
	"sync"
)

// bizProject 默认模板空间的归属维度：业务 + 项目
type bizProject struct {
	bizID     uint32
	projectID uint32
}

// bizsOfTS are biz+project pairs which already have default template spaces
var bizsOfTS = BizsOfTmplSpace{pairs: make(map[bizProject]struct{})}

// BizsOfTmplSpace are biz+project pairs which already have default template spaces
// with a lock which can be used concurrently
type BizsOfTmplSpace struct {
	sync.Mutex
	pairs map[bizProject]struct{}
}

// Set save a biz+project pair in the cache
func (b *BizsOfTmplSpace) Set(bizID, projectID uint32) {
	b.Lock()
	defer b.Unlock()
	b.pairs[bizProject{bizID: bizID, projectID: projectID}] = struct{}{}
}

// Has judge if a biz+project pair in the cache
func (b *BizsOfTmplSpace) Has(bizID, projectID uint32) bool {
	b.Lock()
	defer b.Unlock()
	_, has := b.pairs[bizProject{bizID: bizID, projectID: projectID}]
	return has
}
