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

package render

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderCommandEnvUsesGSEKitTimezone(t *testing.T) {
	t.Setenv("TZ", "UTC")

	env := renderCommandEnv()

	if got := envValue(env, "TZ"); got != "Asia/Shanghai" {
		t.Fatalf("TZ = %q, want Asia/Shanghai", got)
	}
}

func TestNewRendererTimeoutFromEnv(t *testing.T) {
	pythonRoot, err := filepath.Abs("python")
	if err != nil {
		t.Fatalf("failed to resolve python dir: %v", err)
	}
	t.Setenv("BSCP_PYTHON_RENDER_PATH", pythonRoot)

	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{name: "unset falls back to default", env: "", want: defaultTimeoutSec * time.Second},
		{name: "override", env: "600", want: 600 * time.Second},
		{name: "invalid falls back to default", env: "abc", want: defaultTimeoutSec * time.Second},
		{name: "non-positive falls back to default", env: "0", want: defaultTimeoutSec * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(timeoutSecEnv, tt.env)

			r, err := NewRenderer()
			if err != nil {
				t.Fatalf("NewRenderer() error = %v", err)
			}
			if r.timeout != tt.want {
				t.Fatalf("timeout = %v, want %v", r.timeout, tt.want)
			}
		})
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}
