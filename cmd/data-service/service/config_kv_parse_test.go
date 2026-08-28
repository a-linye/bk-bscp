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
	"testing"
)

func TestParsePcvBizIDs(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := parsePcvBizIDs(""); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("sort unique skip invalid", func(t *testing.T) {
		got := parsePcvBizIDs("10, 5,abc,5,0,-1, 3,")
		want := []int{3, 5, 10}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("order independent of source", func(t *testing.T) {
		a := parsePcvBizIDs("9,1,7")
		b := parsePcvBizIDs("1,7,9")
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("sorted lists differ: %v vs %v", a, b)
		}
	})
}
