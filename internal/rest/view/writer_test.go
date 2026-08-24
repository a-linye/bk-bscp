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

package view

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TencentBlueKing/bk-bscp/pkg/rest"
)

// TestGenericResponseWriter_WrapsNormalResponseWithdata 正常 2xx 响应应当被包成 {"data": <body>}
func TestGenericResponseWriter_WrapsNormalResponseWithdata(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewGenericResponseWriter(rec, nil)

	w.WriteHeader(http.StatusOK)
	n, err := w.Write([]byte(`{"foo":"bar"}`))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n == 0 {
		t.Fatalf("no bytes written")
	}

	got := rec.Body.String()
	want := `{"data":{"foo":"bar"}}`
	if got != want {
		t.Fatalf("normal response should be wrapped with data, got=%s, want=%s", got, want)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status code should be preserved, got=%d, want=%d", rec.Code, http.StatusOK)
	}
}

// TestGenericResponseWriter_BadRequestFromChiMiddlewareNotWrapped chi middleware 通过
// render.Render(rest.BadRequest) 直接写出的错误响应, 状态码为 4xx, 不应被包成 {"data": {"error":{...}}}
// 应保持 {"error":{...}} 原样输出。
func TestGenericResponseWriter_BadRequestFromChiMiddlewareNotWrapped(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewGenericResponseWriter(rec, nil)

	// 模拟 rest.BadRequest 写出流程: 先 WriteHeader(4xx), 再 Write 序列化的 ErrorResponse body
	rr := rest.BadRequest(errString("project does not exist"))
	// 复用 rest.ErrorResponse 的 Render 写出: 这里直接构造等价的 json 串, 验证不被二次包装
	_ = rr
	w.WriteHeader(http.StatusBadRequest)
	body := `{"error":{"code":"INVALID_REQUEST","message":"project does not exist","data":null,"details":null}}`
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	got := rec.Body.String()
	if got != body {
		t.Fatalf("bad request from chi middleware should NOT be wrapped with data, got=%s, want=%s", got, body)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code should be preserved, got=%d, want=%d", rec.Code, http.StatusBadRequest)
	}
}

// TestGenericResponseWriter_SetErrorShortCircuits 走 grpc-gateway 错误路径时通过 SetError
// 标记错误, Write 应直接透传, 与状态码兜底行为一致。
func TestGenericResponseWriter_SetErrorShortCircuits(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewGenericResponseWriter(rec, nil)
	w.SetError(errString("rpc failed"))

	body := `{"error":{"code":"INVALID_REQUEST","message":"rpc failed"}}`
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	got := rec.Body.String()
	if got != body {
		t.Fatalf("error response should be passed through, got=%s, want=%s", got, body)
	}
}

// errString 简单的 error 字符串包装, 测试用
type errString string

func (e errString) Error() string { return string(e) }
