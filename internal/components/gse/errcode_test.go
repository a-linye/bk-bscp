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

package gse

import "testing"

func TestIsAlreadyRunning(t *testing.T) {
	cases := []struct {
		name string
		code int
		want bool
	}{
		{"already running", ErrCodeAlreadyRunning, true},
		{"success", ErrCodeSuccess, false},
		{"in progress", ErrCodeInProgress, false},
		{"other", 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsAlreadyRunning(c.code); got != c.want {
				t.Fatalf("IsAlreadyRunning(%d) = %v, want %v", c.code, got, c.want)
			}
		})
	}
}

func TestIsNoNeedStop(t *testing.T) {
	cases := []struct {
		name string
		code int
		want bool
	}{
		{"no need stop", ErrCodeNoNeedStop, true},
		{"already running", ErrCodeAlreadyRunning, false},
		{"success", ErrCodeSuccess, false},
		{"other", 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsNoNeedStop(c.code); got != c.want {
				t.Fatalf("IsNoNeedStop(%d) = %v, want %v", c.code, got, c.want)
			}
		})
	}
}
