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

package job

import (
	"context"
	"fmt"

	"github.com/TencentBlueKing/bk-bscp/internal/components"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
	"github.com/go-resty/resty/v2"
)

// CMDBService bkcmdb client
type Service struct {
	appCode   string
	appSecret string
	host      string
}

// NewService new service
func NewService(appCode, appSecret, host string) *Service {
	return &Service{
		appCode:   appCode,
		appSecret: appSecret,
		host:      host,
	}
}

type HTTPMethod string

const (
	GET  HTTPMethod = "GET"
	POST HTTPMethod = "POST"
)

func (jobService *Service) doRequest(ctx context.Context, method HTTPMethod, url string, body any, result any) error {
	// 组装网关认证信息
	gwAuthOptions := []components.GWAuthOption{}
	withBkUsername := components.WithBkUsername("admin")

	// 多租户模式，带上租户ID
	// if cc.G().FeatureFlags.EnableMultiTenantMode {
	// 	admin, err := bkuser.GetTenantBKAdmin(ctx)
	// 	if err != nil {
	// 		return fmt.Errorf("get tenant admin failed: %w", err)
	// 	}
	// 	withBkUsername = components.WithBkUsername(admin.BkUsername)
	// }
	gwAuthOptions = append(gwAuthOptions, withBkUsername)

	authHeader := components.MakeBKAPIGWAuthHeader(
		jobService.appCode,
		jobService.appSecret,
		gwAuthOptions...,
	)

	// 构造请求
	request := components.GetClient().SetDebug(false).R().
		SetContext(ctx).
		SetHeader("X-Bkapi-Authorization", authHeader).
		SetBody(body)

	// 执行请求
	var resp *resty.Response
	var err error

	switch method {
	case GET:
		resp, err = request.Get(url)
	case POST:
		resp, err = request.Post(url)
	default:
		return fmt.Errorf("%s method not defined", method)
	}

	if err != nil {
		return err
	}

	// 统一反序列化结果，自动处理外层包装和错误码验证
	if err := components.UnmarshalBKResult(resp, result); err != nil {
		logs.Errorf("unmarshal bk result failed, err: %v, resp: %s", err, resp.String())
		return err
	}

	return nil
}
