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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/internal/task/executor/common"
	"github.com/TencentBlueKing/bk-bscp/internal/task/executor/config"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

// ScriptRunner 最小可测 seam：在某 agent 上下发读取 .proc 的脚本并返回 Screen 文本。
// 实现复用 GSE 异步脚本执行 + 结果轮询构建块，不引入 istep 流水线（research D1）。
type ScriptRunner interface {
	// RunProcScript 按 agentID + osType 下发读取 .proc 的命令，返回 Screen 文本。
	RunProcScript(ctx context.Context, agentID, osType string) (string, error)
}

// gseScriptRunner ScriptRunner 的 GSE 实现。
type gseScriptRunner struct {
	exec          *common.Executor
	linuxScript   string
	windowsScript string
}

// NewGSEScriptRunner 构造基于 common.Executor 的 ScriptRunner。
// 仅填充执行所需的 GseService/GseConf/TaskConf，不依赖 istep step/callback。
func NewGSEScriptRunner(gseSvc *gse.Service, linuxScript, windowsScript string) ScriptRunner {
	return &gseScriptRunner{
		exec: &common.Executor{
			GseService: gseSvc,
			GseConf:    cc.G().GSE,
			TaskConf:   cc.G().TaskFramework,
		},
		linuxScript:   linuxScript,
		windowsScript: windowsScript,
	}
}

// RunProcScript 下发 cat .proc 脚本并轮询取 Screen。
// 仅在确实无法取得任何执行结果（任务创建失败/轮询失败/结果为空）时返回 error；
// 脚本退出码非 0 时仍返回带错误信息的文本，交由 ParseProcScreen 区分 agent 异常/解析失败。
func (r *gseScriptRunner) RunProcScript(ctx context.Context, agentID, osType string) (string, error) {
	fileMode := table.Unix
	scriptContent := r.linuxScript
	if osType == "win" {
		fileMode = table.Windows
		scriptContent = r.windowsScript
	}

	ext := ".sh"
	if fileMode == table.Windows {
		ext = ".bat"
	}
	scriptName := fmt.Sprintf("bk_gse_check_proc_%d%s", time.Now().UnixNano(), ext)
	storeDir := config.ScriptStoreDirByFileMode(r.exec.GseConf.ScriptStoreDir, r.exec.GseConf.WindowsScriptStoreDir, fileMode)
	command := config.BuildScriptCommand(storeDir, scriptName, fileMode)

	req := &gse.ExecuteScriptReq{
		Agents: []gse.Agent{
			{
				BkAgentID: agentID,
				// .proc 为 agent 维护文件，以具备权限的账户读取（对标 gsekit ACCOUNT_ALIAS）。
				User: config.GetExecutionUser(fileMode, ""),
			},
		},
		Scripts: []gse.Script{
			{
				ScriptName:     scriptName,
				ScriptStoreDir: storeDir,
				ScriptContent:  scriptContent,
			},
		},
		AtomicTasks: []gse.AtomicTask{
			{
				Command:        command,
				AtomicTaskID:   0,
				TimeoutSeconds: r.exec.TaskConf.ScriptExecution.TimeoutSec,
			},
		},
		AtomicTasksRelations: []gse.AtomicTaskRelation{
			{AtomicTaskID: 0, AtomicTaskIDIdx: []int{}},
		},
	}

	resp, err := r.exec.GseService.AsyncExtensionsExecuteScript(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create execute script task failed, agentID=%s: %w", agentID, err)
	}
	if resp == nil || resp.Result.TaskID == "" {
		return "", fmt.Errorf("gse execute script response is nil, agentID=%s", agentID)
	}

	result, err := r.exec.WaitExecuteScriptFinish(ctx, resp.Result.TaskID, agentID)
	if err != nil {
		return "", fmt.Errorf("wait script execution failed, agentID=%s: %w", agentID, err)
	}
	if len(result.Result) == 0 {
		return "", fmt.Errorf("script execution result is empty, agentID=%s", agentID)
	}

	res := result.Result[0]
	text := res.Screen
	if res.ErrorCode != 0 || res.ScriptExitCode != 0 {
		// 退出异常时把 ErrorMsg 并入文本，便于解析层识别 "agent not available" 等信号。
		text = strings.TrimSpace(text + "\n" + res.ErrorMsg)
	}
	return text, nil
}
