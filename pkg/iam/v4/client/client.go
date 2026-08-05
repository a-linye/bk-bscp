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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

const defaultTimeout = 30 * time.Second

// Client 是 IAM V4 网关的 HTTP 客户端。
type Client struct {
	cfg     *Config
	cli     *resty.Client
	authHdr string
}

// NewClient 构造一个 IAM V4 网关客户端。
func NewClient(cfg *Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	authHdr, err := json.Marshal(map[string]string{
		"bk_app_code":   cfg.AppCode,
		"bk_app_secret": cfg.AppSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal iam v4 auth header failed, err: %v", err)
	}

	return &Client{
		cfg:     cfg,
		cli:     resty.New().SetTimeout(timeout).SetCookieJar(nil),
		authHdr: string(authHdr),
	}, nil
}

// SystemID 返回客户端所属的接入系统 ID。
func (c *Client) SystemID() string {
	return c.cfg.SystemID
}

// request 描述一次网关调用。path 为相对路径，调用方需自行填好 {system_id}。
type request struct {
	method   string
	path     string
	query    map[string]string
	body     any
	operator string
	result   any
}

// apiResponse 同时容纳网关层与应用层两种响应形态。
// 网关层错误形如 {"result":false,"code":1640001,"code_name":"...","message":"..."}，code 是整数；
// 应用层错误形如 {"error":{"code":"...","message":"..."}}，code 是字符串。
// 两者的 code 类型不同，统一用 RawMessage 承接后再归一化。
type apiResponse struct {
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
	Result    *bool           `json:"result"`
	Code      json.RawMessage `json:"code"`
	CodeName  string          `json:"code_name"`
	Message   string          `json:"message"`
	Error     *struct {
		Code    string      `json:"code"`
		Message rawMessages `json:"message"`
	} `json:"error"`
}

// rawMessages 承接应用层错误的 message 字段。它有时是字符串，
// 有时是字符串数组（校验类错误会把多条校验失败一起返回，
// 例如 {"error":{"code":"INVALID_ARGUMENT","message":["同一资源树（biz）下的展示层级必须一致"]}}）。
type rawMessages []string

// UnmarshalJSON implements json.Unmarshaler.
func (m *rawMessages) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*m = []string{single}

		return nil
	}

	var multi []string
	if err := json.Unmarshal(data, &multi); err != nil {
		return err
	}
	*m = multi

	return nil
}

// String joins all messages into a single line.
func (m rawMessages) String() string {
	return strings.Join(m, "; ")
}

func (c *Client) do(kt *kit.Kit, r *request) error {
	req := c.cli.R().
		SetContext(kt.Ctx).
		SetHeader(authorizationHeader, c.authHdr).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")

	if kt.TenantID != "" {
		req.SetHeader(tenantIDHeader, kt.TenantID)
	}

	if r.operator != "" {
		req.SetHeader(operatorHeader, r.operator)
	}

	if len(r.query) != 0 {
		req.SetQueryParams(r.query)
	}

	if r.body != nil {
		req.SetBody(r.body)
	}

	url := strings.TrimSuffix(c.cfg.GatewayURL, "/") + r.path
	resp, err := req.Execute(r.method, url)
	if err != nil {
		return fmt.Errorf("request iam v4 %s %s failed, err: %v", r.method, r.path, err)
	}

	return handleResponse(resp, r.result)
}

func handleResponse(resp *resty.Response, result any) error {
	status := resp.StatusCode()
	body := resp.Body()
	reqID := resp.Header().Get(requestIDHeader)

	// 空响应体只在成功时合法，204 与部分 201 属于这种情况。
	if len(body) == 0 {
		if isSuccessStatus(status) {
			return nil
		}

		return &Error{Layer: ErrorLayerGateway, HTTPStatus: status, RequestID: reqID,
			Message: "empty response body"}
	}

	parsed := new(apiResponse)
	if err := json.Unmarshal(body, parsed); err != nil {
		return &Error{Layer: ErrorLayerGateway, HTTPStatus: status, RequestID: reqID,
			Message: fmt.Sprintf("unmarshal response failed: %v, body: %s", err, string(body))}
	}

	if parsed.RequestID != "" {
		reqID = parsed.RequestID
	}

	// 应用层错误带独立的 error 字段，与网关层的 result/code 互斥，优先判定。
	if parsed.Error != nil {
		return &Error{Layer: ErrorLayerApp, HTTPStatus: status, RequestID: reqID,
			Code: parsed.Error.Code, Message: parsed.Error.Message.String()}
	}

	if !isSuccessStatus(status) {
		return &Error{Layer: ErrorLayerGateway, HTTPStatus: status, RequestID: reqID,
			Code: normalizeCode(parsed.Code), Message: parsed.Message}
	}

	if result == nil || len(parsed.Data) == 0 || string(parsed.Data) == "null" {
		return nil
	}

	if err := json.Unmarshal(parsed.Data, result); err != nil {
		return &Error{Layer: ErrorLayerApp, HTTPStatus: status, RequestID: reqID,
			Message: fmt.Sprintf("unmarshal data failed: %v, data: %s", err, string(parsed.Data))}
	}

	return nil
}

// isSuccessStatus 判断状态码是否表示成功。IAM V4 的创建类返回 201，
// 全量更新与删除类返回 204 且无响应体，查询类返回 200，不能只判 200。
func isSuccessStatus(status int) bool {
	return status == http.StatusOK || status == http.StatusCreated || status == http.StatusNoContent
}

// normalizeCode 把网关层的整数 code 与应用层的字符串 code 统一成字符串。
func normalizeCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	return strings.Trim(string(raw), `"`)
}
