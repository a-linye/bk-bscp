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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

func testKit() *kit.Kit {
	return &kit.Kit{Ctx: context.Background(), TenantID: "default", User: "tester"}
}

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cli, err := NewClient(&Config{
		GatewayURL: srv.URL,
		SystemID:   "bk-bscp-test",
		AppCode:    "code",
		AppSecret:  "secret",
	})
	require.NoError(t, err)

	return cli
}

func TestNewClientValidatesConfig(t *testing.T) {
	_, err := NewClient(&Config{SystemID: "s", AppCode: "c", AppSecret: "s"})
	require.ErrorContains(t, err, "gateway url")

	_, err = NewClient(&Config{GatewayURL: "http://x", AppCode: "c", AppSecret: "s"})
	require.ErrorContains(t, err, "system id")

	_, err = NewClient(&Config{GatewayURL: "http://x", SystemID: "s", AppSecret: "s"})
	require.ErrorContains(t, err, "app code")

	_, err = NewClient(&Config{GatewayURL: "http://x", SystemID: "s", AppCode: "c"})
	require.ErrorContains(t, err, "app secret")
}

func TestDoSuccess200(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, `{"bk_app_code":"code","bk_app_secret":"secret"}`,
			r.Header.Get(authorizationHeader))
		require.Equal(t, "default", r.Header.Get(tenantIDHeader))
		require.Empty(t, r.Header.Get(operatorHeader))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"bk-bscp-test"},"request_id":"req-1"}`))
	})

	var got struct {
		ID string `json:"id"`
	}
	err := cli.do(testKit(), &request{method: http.MethodGet, path: "/x", result: &got})
	require.NoError(t, err)
	require.Equal(t, "bk-bscp-test", got.ID)
}

func TestDoSuccess201(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":["a","b"],"request_id":"req-2"}`))
	})

	var got []string
	err := cli.do(testKit(), &request{method: http.MethodPost, path: "/x",
		body: []string{}, result: &got})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got)
}

func TestDoSuccess204NoBody(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	err := cli.do(testKit(), &request{method: http.MethodDelete, path: "/x"})
	require.NoError(t, err)
}

func TestDoGatewayError(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"result":false,"code":1640001,"code_name":"INVALID_ARGS",` +
			`"message":"Parameters error","data":null}`))
	})

	err := cli.do(testKit(), &request{method: http.MethodGet, path: "/x"})
	require.Error(t, err)

	var e *Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, ErrorLayerGateway, e.Layer)
	require.Equal(t, "1640001", e.Code)
	require.Equal(t, http.StatusBadRequest, e.HTTPStatus)
}

func TestDoAppError(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(requestIDHeader, "req-hdr")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"NO_PERMISSION","message":"denied"}}`))
	})

	err := cli.do(testKit(), &request{method: http.MethodGet, path: "/x"})
	require.Error(t, err)

	var e *Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, ErrorLayerApp, e.Layer)
	require.Equal(t, "NO_PERMISSION", e.Code)
	require.Equal(t, "req-hdr", e.RequestID)
}

func TestDoNotFound(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"result":false,"code":1640404,"message":"not found"}`))
	})

	err := cli.do(testKit(), &request{method: http.MethodGet, path: "/x"})
	require.True(t, IsNotFound(err))
}

func TestDoOperatorHeader(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "alice", r.Header.Get(operatorHeader))
		w.WriteHeader(http.StatusNoContent)
	})

	err := cli.do(testKit(), &request{method: http.MethodPost, path: "/x", operator: "alice"})
	require.NoError(t, err)
}

// 响应体的 request_id 优先于响应头，两者都为空时不影响错误构造。
func TestDoRequestIDFromBody(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(requestIDHeader, "from-header")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"result":false,"code":1640001,"message":"x","request_id":"from-body"}`))
	})

	err := cli.do(testKit(), &request{method: http.MethodGet, path: "/x"})

	var e *Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, "from-body", e.RequestID)
}

// data 为 null 时不应尝试反序列化，避免把 result 写坏。
func TestDoNullData(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":null,"request_id":"r"}`))
	})

	got := []string{"untouched"}
	err := cli.do(testKit(), &request{method: http.MethodGet, path: "/x", result: &got})
	require.NoError(t, err)
	require.Equal(t, []string{"untouched"}, got)
}

func TestDoQueryParams(t *testing.T) {
	cli := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "2", r.URL.Query().Get("page"))
		require.Equal(t, "50", r.URL.Query().Get("page_size"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{},"request_id":"r"}`))
	})

	err := cli.do(testKit(), &request{method: http.MethodGet, path: "/x",
		query: map[string]string{"page": "2", "page_size": "50"}})
	require.NoError(t, err)
}
