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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitResBaseJSURL(t *testing.T) {
	conf := defaultUIConf()
	err := conf.initResBaseJSURL("bk-bscp")
	assert.NoError(t, err)
	assert.Equal(t, "", conf.Host.BKSharedResBaseJSURL)

	conf.Host.BKSharedResURL = "http://repo.test.com"
	err = conf.initResBaseJSURL("bk-bscp")
	assert.NoError(t, err)
	assert.Equal(t, "http://repo.test.com/bk_bscp/base.js", conf.Host.BKSharedResBaseJSURL)
}

func TestHostConfGetFromEnv(t *testing.T) {
	// 字段为空时, 从环境变量补充
	t.Run("fill from env when empty", func(t *testing.T) {
		t.Setenv(NewUIURLEnv, "https://new-ui.example.com")
		t.Setenv(OldUIURLEnv, "https://old-ui.example.com")

		h := &HostConf{}
		h.getFromEnv()
		assert.Equal(t, "https://new-ui.example.com", h.NewUIURL)
		assert.Equal(t, "https://old-ui.example.com", h.OldUIURL)
	})

	// 字段非空时, 显式配置优先, 不被环境变量覆盖
	t.Run("keep explicit config", func(t *testing.T) {
		t.Setenv(NewUIURLEnv, "https://new-ui.example.com")
		t.Setenv(OldUIURLEnv, "https://old-ui.example.com")

		h := &HostConf{
			NewUIURL: "https://explicit-new.example.com",
			OldUIURL: "https://explicit-old.example.com",
		}
		h.getFromEnv()
		assert.Equal(t, "https://explicit-new.example.com", h.NewUIURL)
		assert.Equal(t, "https://explicit-old.example.com", h.OldUIURL)
	})
}
