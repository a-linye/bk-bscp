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

package bkpaas

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TencentBlueKing/bk-bscp/internal/components"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/constant"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetUserInfoByToken(t *testing.T) {
	restyClient := components.GetClient()
	oldTransport := restyClient.GetClient().Transport
	restyClient.SetTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/user/is_login/", r.URL.Path)
		require.Equal(t, "test-ticket", r.URL.Query().Get("bk_ticket"))

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ret":0,"msg":"ok"}`)),
		}, nil
	}))
	defer restyClient.SetTransport(oldTransport)

	client := NewAuthLoginClient(&cc.LoginAuthSettings{Provider: BKLoginProvider})
	username, err := client.GetUserInfoByToken(context.Background(), "https://bklogin.example.com", "dummyUser", "test-ticket")
	require.NoError(t, err)
	require.Equal(t, "dummyUser", username)
}

func TestGetTenantUserInfoByTokenUsesUserInfoEndpoint(t *testing.T) {
	cc.SetG(cc.GlobalSettings{
		Esb: cc.Esb{
			Endpoints: []string{"https://bkapi.example.com"},
			AppCode:   "test-app-code",
			AppSecret: "test-app-secret",
		},
	})

	client := components.GetClient()
	oldTransport := client.GetClient().Transport
	client.SetTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/bk-login/prod/login/api/v3/open/bk-tokens/userinfo/", r.URL.Path)
		require.Equal(t, "test-token", r.URL.Query().Get("bk_token"))
		require.Equal(t, constant.DefaultTenantID, r.Header.Get("X-Bk-Tenant-Id"))
		require.NotEmpty(t, r.Header.Get("X-Bkapi-Authorization"))

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"code": 0,
				"message": "ok",
				"data": {
					"bk_username": "xiaolnwang",
					"tenant_id": "tencent",
					"time_zone": "Asia/Shanghai"
				}
			}`)),
		}, nil
	}))
	defer client.SetTransport(oldTransport)

	authClient := NewAuthLoginClient(&cc.LoginAuthSettings{Provider: BKPaaSProvider})
	info, err := authClient.GetTenantUserInfoByToken(context.Background(), "test-token")
	require.NoError(t, err)
	require.Equal(t, "xiaolnwang", info.BkUsername)
	require.Equal(t, "tencent", info.TenantID)
	require.Equal(t, "Asia/Shanghai", info.TimeZone)
}
