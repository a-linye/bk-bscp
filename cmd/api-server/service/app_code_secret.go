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
	"net/http"

	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/constant"
	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/errf"
	"github.com/TencentBlueKing/bk-bscp/pkg/rest"
)

// AppCodeSecretVerified is a middleware that verifies the request carries valid
// X-Bkapi-App-Code and X-Bkapi-App-Secret headers matching BSCP's own credentials.
func (p *proxy) AppCodeSecretVerified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appCode := r.Header.Get(constant.AppCodeKey)
		appSecret := r.Header.Get(constant.AppSecretKey)

		baseConf := cc.G().BaseConf
		if appCode == "" || appSecret == "" {
			w.WriteHeader(http.StatusUnauthorized)
			rest.WriteResp(w, rest.NewBaseResp(errf.Unauthenticated,
				"missing X-Bkapi-App-Code or X-Bkapi-App-Secret header"))
			return
		}

		if appCode != baseConf.AppCode || appSecret != baseConf.AppSecret {
			w.WriteHeader(http.StatusForbidden)
			rest.WriteResp(w, rest.NewBaseResp(errf.NoPermission, "invalid app credentials"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
