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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	pbas "github.com/TencentBlueKing/bk-bscp/pkg/protocol/auth-server"
	pbapp "github.com/TencentBlueKing/bk-bscp/pkg/protocol/core/app"
	pbcs "github.com/TencentBlueKing/bk-bscp/pkg/protocol/config-server"
)

// stubCfgClient 嵌入 ConfigClient，只覆写 EnsureDefaultProjectEnv。
type stubCfgClient struct {
	pbcs.ConfigClient

	ensureResp *pbcs.EnsureDefaultProjectEnvResp
	ensureErr  error
	getAppErr  error
}

func (s *stubCfgClient) EnsureDefaultProjectEnv(ctx context.Context, in *pbcs.EnsureDefaultProjectEnvReq,
	opts ...grpc.CallOption) (*pbcs.EnsureDefaultProjectEnvResp, error) {
	return s.ensureResp, s.ensureErr
}

func (s *stubCfgClient) GetApp(ctx context.Context, in *pbcs.GetAppReq, opts ...grpc.CallOption) (*pbapp.App, error) {
	if s.getAppErr != nil {
		return nil, s.getAppErr
	}
	return &pbapp.App{Id: in.GetAppId()}, nil
}

func permissionDeniedWithApplyURL(applyURL string) error {
	st, err := status.New(codes.PermissionDenied, "permission denied").WithDetails(&pbas.ApplyDetail{
		ApplyUrl: applyURL,
		Resources: []*pbas.BasicDetail{{
			Type:       "biz",
			Action:     "find_business_resource",
			ResourceId: "1",
		}},
	})
	if err != nil {
		panic(err)
	}
	return st.Err()
}

func newDefaultProjectEnvRequest(bizID uint32) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/biz/1/projects/list", nil)
	kt := &kit.Kit{Ctx: context.Background(), BizID: bizID}
	ctx := kit.WithKit(req.Context(), kt)
	ctx = context.WithValue(ctx, render.ContentTypeCtxKey, render.ContentTypeJSON)
	return req.WithContext(ctx)
}

// TestCheckOrCreateDefaultProjectEnv_PermissionDeniedKeepsApplyURL 无权限时必须返回 403，
// 并保留申请链接，供前端弹出权限申请页。
func TestCheckOrCreateDefaultProjectEnv_PermissionDeniedKeepsApplyURL(t *testing.T) {
	const applyURL = "https://iam.example.com/apply"
	p := &proxy{cfgClient: &stubCfgClient{ensureErr: permissionDeniedWithApplyURL(applyURL)}}

	nextCalled := false
	handler := p.checkOrCreateDefaultProjectEnv(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, newDefaultProjectEnvRequest(1))

	if nextCalled {
		t.Fatal("permission denied should stop the request before the next handler")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
			Data struct {
				ApplyURL string `json:"apply_url"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", rr.Body.String(), err)
	}
	if body.Error.Code != "PERMISSION_DENIED" {
		t.Fatalf("code = %s, want PERMISSION_DENIED, body = %s", body.Error.Code, rr.Body.String())
	}
	if body.Error.Data.ApplyURL != applyURL {
		t.Fatalf("apply_url = %q, want %q, body = %s", body.Error.Data.ApplyURL, applyURL, rr.Body.String())
	}
}

func newAppProjectEnvRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/biz/1/projects/2/envs/3/apps/9", nil)
	kt := &kit.Kit{Ctx: context.Background(), BizID: 1, ProjectID: 2, EnvID: 3}
	ctx := kit.WithKit(req.Context(), kt)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("app_id", "9")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, render.ContentTypeCtxKey, render.ContentTypeJSON)
	return req.WithContext(ctx)
}

func assertApplyURL(t *testing.T, rr *httptest.ResponseRecorder, applyURL string) {
	t.Helper()
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
			Data struct {
				ApplyURL string `json:"apply_url"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", rr.Body.String(), err)
	}
	if body.Error.Code != "PERMISSION_DENIED" {
		t.Fatalf("code = %s, want PERMISSION_DENIED, body = %s", body.Error.Code, rr.Body.String())
	}
	if body.Error.Data.ApplyURL != applyURL {
		t.Fatalf("apply_url = %q, want %q, body = %s", body.Error.Data.ApplyURL, applyURL, rr.Body.String())
	}
}

// TestAppProjectEnvVerified_PermissionDeniedKeepsApplyURL 服务归属校验遇到无权限时必须返回 403
// 和申请链接。GetApp 同时校验业务访问与服务查看，缺服务查看时不能改写成「不属于该项目」。
func TestAppProjectEnvVerified_PermissionDeniedKeepsApplyURL(t *testing.T) {
	const applyURL = "https://iam.example.com/apply-app"
	p := &proxy{cfgClient: &stubCfgClient{getAppErr: permissionDeniedWithApplyURL(applyURL)}}

	nextCalled := false
	handler := p.AppProjectEnvVerified(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, newAppProjectEnvRequest())

	if nextCalled {
		t.Fatal("permission denied should stop the request before the next handler")
	}
	assertApplyURL(t, rr, applyURL)
}

// TestAppProjectEnvVerified_OtherErrorStaysOwnership 服务不存在等非鉴权失败仍返回归属错误，不带申请链接。
func TestAppProjectEnvVerified_OtherErrorStaysOwnership(t *testing.T) {
	p := &proxy{cfgClient: &stubCfgClient{getAppErr: errors.New("app not found")}}

	rr := httptest.NewRecorder()
	p.AppProjectEnvVerified(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ownership failure should stop the request")
	})).ServeHTTP(rr, newAppProjectEnvRequest())

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", rr.Body.String(), err)
	}
	if body.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("code = %s, body = %s", body.Error.Code, rr.Body.String())
	}
	if body.Error.Message != "app does not belong to the specified project or environment" {
		t.Fatalf("message = %q, body = %s", body.Error.Message, rr.Body.String())
	}
}
