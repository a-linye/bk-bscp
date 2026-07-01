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

package processcheck

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/internal/task/executor/common"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// TestGSEScriptRunner_RunProcScript_Integration 真连 GSE 验证 RunProcScript 的接口返回。
//
// 该用例默认跳过（不影响 CI / 常规 go test），需要通过环境变量提供真实的蓝鲸 GSE
// 网关凭据与目标 agent 后手动运行，用于验证「下发 cat .proc 脚本 → 轮询取 Screen」
// 这条真实链路能拿到接口返回：
//
//	BSCP_TEST_GSE_HOST=https://bkapi.example.com/api/bk-gse/prod \
//	BSCP_TEST_GSE_APP_CODE=<bk_app_code> \
//	BSCP_TEST_GSE_APP_SECRET=<bk_app_secret> \
//	BSCP_TEST_GSE_AGENT_ID=<目标 agent id> \
//	go test ./internal/processor/processcheck/ \
//	    -run TestGSEScriptRunner_RunProcScript_Integration -v -count=1
//
// 可选环境变量：
//   - BSCP_TEST_TENANT_ID   租户 ID（默认 default），作为 X-Bk-Tenant-Id 传给网关
//   - BSCP_TEST_OS_TYPE     目标机器系统类型 linux/win（默认 linux）
//   - BSCP_TEST_PROC_SCRIPT 覆盖读取 .proc 的脚本内容（默认 cat 默认路径）
//   - BSCP_TEST_BIZ_ID      用于把 Screen 交给 ParseProcScreen 过滤本业务托管项的 bizID
func TestGSEScriptRunner_RunProcScript_Integration(t *testing.T) {
	host := os.Getenv("BSCP_TEST_GSE_HOST")
	appCode := os.Getenv("BSCP_TEST_GSE_APP_CODE")
	appSecret := os.Getenv("BSCP_TEST_GSE_APP_SECRET")
	agentID := os.Getenv("BSCP_TEST_GSE_AGENT_ID")
	if host == "" || appCode == "" || appSecret == "" || agentID == "" {
		t.Skip("跳过 GSE 集成测试：需设置 BSCP_TEST_GSE_HOST/APP_CODE/APP_SECRET/AGENT_ID 后手动运行")
	}

	tenantID := os.Getenv("BSCP_TEST_TENANT_ID")
	if tenantID == "" {
		tenantID = "default"
	}
	osType := os.Getenv("BSCP_TEST_OS_TYPE")
	if osType == "" {
		osType = "linux"
	}
	linuxScript := os.Getenv("BSCP_TEST_PROC_SCRIPT")
	if linuxScript == "" {
		linuxScript = "cat /usr/local/gse/proxy/etc/.proc"
	}

	// 直接构造 gseScriptRunner，绕开 NewGSEScriptRunner 对全局 cc.G() 的依赖，
	// 使集成测试不必先加载完整配置文件即可运行。
	runner := &gseScriptRunner{
		exec: &common.Executor{
			GseService: gse.NewService(appCode, appSecret, host),
			GseConf: cc.GSE{
				ScriptStoreDir:        "/tmp/bkbscp",
				WindowsScriptStoreDir: `c:\tmp\bkbscp\Administrator`,
			},
			TaskConf: cc.TaskFramework{
				ScriptExecution: cc.ScriptExecutionConfig{
					TimeoutSec:   180,
					PollTimeout:  240 * time.Second,
					PollInterval: 2 * time.Second,
				},
			},
		},
		linuxScript:   linuxScript,
		windowsScript: `type c:\gse2_bkte\agent\etc\.proc`,
	}

	kt := kit.NewWithTenant(tenantID)
	screen, err := runner.RunProcScript(kt.Ctx, agentID, osType)
	if err != nil {
		t.Fatalf("RunProcScript 调用失败, agentID=%s: %v", agentID, err)
	}
	t.Logf("RunProcScript 返回 Screen（%d 字节）:\n%s", len(screen), screen)

	if strings.TrimSpace(screen) == "" {
		t.Fatalf("期望从 agent %s 拿到非空 Screen，实际为空", agentID)
	}

	// 附带验证：真实 .proc（JSON）应能被 ParseProcScreen 解析。
	// 若目标 agent 无本业务托管项，解析结果可能为空但不应报错；
	// 若 agent 异常/脚本失败，接口仍返回带错误信息的文本，此处仅记录不判失败。
	bizID := sampleBizID
	if v := os.Getenv("BSCP_TEST_BIZ_ID"); v != "" {
		if n, perr := strconv.ParseUint(v, 10, 32); perr == nil {
			bizID = uint32(n)
		}
	}
	actual, perr := ParseProcScreen(screen, bizID)
	if perr != nil {
		t.Logf("ParseProcScreen 未通过（可能是 agent 异常或非本业务机器）: %v", perr)
		return
	}
	t.Logf("ParseProcScreen 解析出 %d 条本业务(bizID=%d)托管项", len(actual), bizID)
}
