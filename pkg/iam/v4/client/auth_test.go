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

package client

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildIAMPath(t *testing.T) {
	require.Equal(t, "/biz,100001/", BuildIAMPath("biz", "100001"))
}

func TestNewAuthResourceOmitsEmptyPath(t *testing.T) {
	require.Nil(t, NewAuthResource("30001", "").Attributes)
	require.Equal(t, map[string]string{IAMPathAttrKey: "/biz,1/"},
		NewAuthResource("30001", "/biz,1/").Attributes)
}

func TestAuthCarriesAttributes(t *testing.T) {
	var captured map[string]any
	cli := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &captured))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"allowed":true},"request_id":"r"}`))
	})

	res := NewAuthResource("30001", BuildIAMPath("biz", "100001"))
	allowed, err := cli.Auth(testKit(), NewUserSubject("tester"), "app_view", &res)
	require.NoError(t, err)
	require.True(t, allowed)

	resource := captured["resource"].(map[string]any)
	attrs := resource["attributes"].(map[string]any)
	require.Equal(t, "/biz,100001/", attrs[IAMPathAttrKey])
}

func TestAuthWithoutResourceOmitsField(t *testing.T) {
	var captured map[string]any
	cli := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &captured))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"allowed":false},"request_id":"r"}`))
	})

	allowed, err := cli.Auth(testKit(), NewUserSubject("tester"), "app_create", nil)
	require.NoError(t, err)
	require.False(t, allowed)
	require.NotContains(t, captured, "resource")
}

func TestAuthByResourcesMapsByResourceID(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"resource_id":"1","allowed":true},` +
			`{"resource_id":"2","allowed":false}]}`))
	})

	got, err := cli.AuthByResources(testKit(), NewUserSubject("tester"), "app_view",
		[]AuthResource{NewAuthResource("1", "/biz,100001/"), NewAuthResource("2", "/biz,100001/")})
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"1": true, "2": false}, got)
}

func TestAuthByActionsMapsByActionID(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"action_id":"app_view","allowed":true},` +
			`{"action_id":"app_edit","allowed":false}]}`))
	})

	res := NewAuthResource("30001", "/biz,100001/")
	got, err := cli.AuthByActions(testKit(), NewUserSubject("tester"),
		[]string{"app_view", "app_edit"}, &res)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"app_view": true, "app_edit": false}, got)
}

// 超过单次上限时客户端直接报错，不隐式截断也不自动分批：
// 分批与并发是上层鉴权逻辑的职责，客户端只做协议映射。
func TestAuthBatchSizeLimit(t *testing.T) {
	cli := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("request should not reach the server")
	})

	resources := make([]AuthResource, MaxAuthBatchSize+1)
	_, err := cli.AuthByResources(testKit(), NewUserSubject("t"), "app_view", resources)
	require.ErrorContains(t, err, "exceeds")

	actions := make([]string, MaxAuthBatchSize+1)
	_, err = cli.AuthByActions(testKit(), NewUserSubject("t"), actions, nil)
	require.ErrorContains(t, err, "exceeds")
}
