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

package keys

import "testing"

// TestAppIDKeyIsolation 验证 app-id 缓存 key 带项目/环境维度，且维度为零时保持旧格式。
func TestAppIDKeyIsolation(t *testing.T) {
	legacy := Key.AppID(1, 0, 0, "app")
	if legacy != "{1}bscp:app-id:app" {
		t.Fatalf("legacy app id key format changed, got: %s", legacy)
	}

	withDims := Key.AppID(1, 2, 3, "app")
	if withDims != "{1}bscp:app-id:2:3:app" {
		t.Fatalf("app id key should contain project/env dims, got: %s", withDims)
	}

	if Key.AppID(1, 2, 3, "app") == Key.AppID(1, 2, 4, "app") {
		t.Fatal("app id key collides across envs")
	}
	if Key.AppID(1, 2, 3, "app") == Key.AppID(1, 3, 3, "app") {
		t.Fatal("app id key collides across projects")
	}
	if Key.AppID(1, 2, 3, "app") == Key.AppID(1, 0, 0, "app") {
		t.Fatal("app id key collides between legacy and project/env scope")
	}
}

// TestAppIDLockKeyIsolation 验证 app-id 刷新锁键与缓存键使用相同的 project/env 作用域维度，
// 避免不同项目/环境下的同名应用争抢同一把锁导致误报 RecordNotFound。
func TestAppIDLockKeyIsolation(t *testing.T) {
	if ResKind.AppID(1, 2, 3, "app") == ResKind.AppID(1, 2, 4, "app") {
		t.Fatal("app id lock key collides across envs")
	}
	if ResKind.AppID(1, 2, 3, "app") == ResKind.AppID(1, 3, 3, "app") {
		t.Fatal("app id lock key collides across projects")
	}
	if ResKind.AppID(1, 2, 3, "app") != "app-id-1-2-3-app" {
		t.Fatalf("unexpected app id lock key format, got: %s", ResKind.AppID(1, 2, 3, "app"))
	}
}

// TestCredentialKeyIsolation 验证 credential 缓存 key 带项目维度，且维度为零时保持旧格式。
func TestCredentialKeyIsolation(t *testing.T) {
	legacy := Key.Credential(1, 0, "tk")
	if legacy != "{1}bscp:credential:tk" {
		t.Fatalf("legacy credential key format changed, got: %s", legacy)
	}

	withDims := Key.Credential(1, 2, "tk")
	if withDims != "{1}bscp:credential:2:0:tk" {
		t.Fatalf("credential key should contain project dim, got: %s", withDims)
	}

	if Key.Credential(1, 2, "tk") == Key.Credential(1, 3, "tk") {
		t.Fatal("credential key collides across projects")
	}
	if Key.Credential(1, 2, "tk") == Key.Credential(1, 0, "tk") {
		t.Fatal("credential key collides between legacy and project scope")
	}
}

// TestDimensionFreeKeysUnchanged 验证不带项目/环境维度的 key 格式不受影响。
func TestDimensionFreeKeysUnchanged(t *testing.T) {
	if got := Key.ReleasedCI(1, 2); got != "{1}bscp:released-ci:2" {
		t.Fatalf("released-ci key format changed, got: %s", got)
	}
	if got := Key.AppMeta(1, 2); got != "{1}bscp:app-meta:2" {
		t.Fatalf("app-meta key format changed, got: %s", got)
	}
	if got := Key.CredentialMatchedCI(1, "tk"); got != "{1}bscp:credential-matched-ci:tk" {
		t.Fatalf("credential-matched-ci key format changed, got: %s", got)
	}
}
