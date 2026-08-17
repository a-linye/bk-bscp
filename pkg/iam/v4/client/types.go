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

// Package client provides the HTTP client for the BK-IAM V4 gateway.
package client

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// 批量接口的单次请求上限。
const (
	// MaxAuthBatchSize 批量鉴权单次请求可携带的资源数或操作数上限。
	MaxAuthBatchSize = 20
	// MaxAuthorizationBatchSize 授权与撤销授权单次请求可携带的条目数上限。
	MaxAuthorizationBatchSize = 20
	// MaxPageSize 模型查询类接口的分页大小上限。
	MaxPageSize = 100
)

// 请求头名称。
const (
	authorizationHeader = "X-Bkapi-Authorization"
	requestIDHeader     = "X-Bkapi-Request-Id"
	operatorHeader      = "X-Bkiam-Operator"
	tenantIDHeader      = "X-Bk-Tenant-Id"
)

// SubjectTypeUser 鉴权与授权接口中表示用户主体的类型值。
const SubjectTypeUser = "user"

// CallbackBasicUser 是权限中心回调接入系统时 Basic 认证的用户名，IAM V4 固定为 bk_iam。
// V3 用的是 iam，两者不同，回调认证的校验必须按版本区分。
const CallbackBasicUser = "bk_iam"

// Config 是 IAM V4 网关客户端的配置。
type Config struct {
	// GatewayURL 网关地址
	GatewayURL string
	// SystemID 接入系统的 ID
	SystemID string
	// AppCode 调用方的蓝鲸应用 ID
	AppCode string
	// AppSecret 调用方的蓝鲸应用密钥
	AppSecret string
	// Timeout 单次请求超时，为零时取 defaultTimeout
	Timeout time.Duration
}

func (c *Config) validate() error {
	if c.GatewayURL == "" {
		return errors.New("gateway url is not set")
	}

	if c.SystemID == "" {
		return errors.New("system id is not set")
	}

	if c.AppCode == "" {
		return errors.New("app code is not set")
	}

	if c.AppSecret == "" {
		return errors.New("app secret is not set")
	}

	return nil
}

// Subject 鉴权与授权的主体。
type Subject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// NewUserSubject 构造一个用户类型的主体。
func NewUserSubject(username string) Subject {
	return Subject{Type: SubjectTypeUser, ID: username}
}

// ErrorLayer 标识错误来自网关层还是权限中心应用层，两者的错误码体系不同。
type ErrorLayer string

// 错误来源。
const (
	// ErrorLayerGateway 网关层错误，错误码形如 1640001。
	ErrorLayerGateway ErrorLayer = "gateway"
	// ErrorLayerApp 权限中心应用层错误。
	ErrorLayerApp ErrorLayer = "app"
)

// Error 是 IAM V4 网关返回的错误。
type Error struct {
	// Layer 错误来源，用于区分网关层与应用层两套错误码
	Layer ErrorLayer
	// HTTPStatus 响应状态码
	HTTPStatus int
	// Code 错误码，网关层为整数形式的字符串，应用层为字符串枚举
	Code string
	// Message 错误描述
	Message string
	// RequestID 权限中心的请求 ID，报障时需提供
	RequestID string
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("iam v4 %s error, http status: %d, code: %s, message: %s, request id: %s",
		e.Layer, e.HTTPStatus, e.Code, e.Message, e.RequestID)
}

// IsNotFound 判断错误是否为资源不存在，用于区分"系统未注册"与真正的调用失败。
func IsNotFound(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.HTTPStatus == http.StatusNotFound
	}

	return false
}
