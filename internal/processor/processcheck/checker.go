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
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
)

// checkConcurrency 以 host(agentID) 为并发单元的上限
const checkConcurrency = 10

// Checker 进程托管配置检查器
type Checker struct {
	set     dao.Set
	runner  ScriptRunner
	limiter *rate.Limiter
}

// NewChecker 构造进程托管配置检查器
func NewChecker(set dao.Set, runner ScriptRunner, limiter *rate.Limiter) *Checker {
	return &Checker{set: set, runner: runner, limiter: limiter}
}

// CheckBiz 检查进程托管配置
func (c *Checker) CheckBiz(kt *kit.Kit, bizID uint32) error {
	processes, err := c.set.Process().ListProcessesWithInstance(kt, bizID)
	if err != nil {
		return err
	}
	if len(processes) == 0 {
		return nil
	}

	processIDs := make([]uint32, 0, len(processes))
	for _, p := range processes {
		processIDs = append(processIDs, p.ID)
	}
	insts, err := c.set.ProcessInstance().GetByProcessIDs(kt, bizID, processIDs)
	if err != nil {
		return err
	}
	if len(insts) == 0 {
		return nil
	}

	instsByProcess := make(map[uint32][]*table.ProcessInstance, len(processes))
	for _, inst := range insts {
		if inst == nil || inst.Attachment == nil {
			continue
		}
		instsByProcess[inst.Attachment.ProcessID] = append(instsByProcess[inst.Attachment.ProcessID], inst)
	}

	expected := make([]ExpectedProc, 0, len(insts))
	for _, p := range processes {
		pInsts := instsByProcess[p.ID]
		if len(pInsts) == 0 {
			continue
		}
		// agentID 为空的异常进程记录无法下发脚本
		if p.Attachment == nil || p.Attachment.AgentID == "" {
			continue
		}
		// agent 状态非 normal 时无法下发脚本
		if p.Spec == nil || p.Spec.AgentStatus != table.AgentStatusNormal {
			continue
		}
		expected = append(expected, BuildExpectedProcs(p, pInsts, bizID)...)
	}
	if len(expected) == 0 {
		return nil
	}

	runChecks(kt, bizID, expected, c.runner, c.set.ProcessManagedException(), c.limiter, time.Now().UTC())
	return nil
}

// runChecks 检查编排核心（可单测）：按 agent 分组并发下发 → 解析（含 contact 过滤）→ 比对/host 级错误扇出 → 逐实例落库。
// 单主机下发/解析失败仅扇出对应 error_type 并继续；单实例写库失败仅记日志，不阻断其余（FR-010/FR-012）。
func runChecks(kt *kit.Kit, bizID uint32, expected []ExpectedProc, runner ScriptRunner,
	store dao.ProcessManagedException, limiter *rate.Limiter, checkedAt time.Time) {

	byAgent := make(map[string][]ExpectedProc)
	for _, e := range expected {
		byAgent[e.AgentID] = append(byAgent[e.AgentID], e)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []CheckResult
		sem     = make(chan struct{}, checkConcurrency)
	)

	for agentID, exp := range byAgent {
		wg.Add(1)
		go func(agentID string, exp []ExpectedProc) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					logs.Errorf("[checkManagedProcess] biz %d agent %s panic recovered: %v", bizID, agentID, rec)
				}
			}()

			sem <- struct{}{}
			defer func() { <-sem }()

			hostResults := checkAgent(kt, bizID, agentID, exp, runner, limiter, checkedAt)

			mu.Lock()
			results = append(results, hostResults...)
			mu.Unlock()
		}(agentID, exp)
	}
	wg.Wait()

	for _, r := range results {
		if err := ApplyResult(kt, store, r); err != nil {
			logs.Errorf("[checkManagedProcess] biz %d write result failed, instID=%d, verdict=%s, err=%v",
				bizID, r.ProcessInstanceID, r.Verdict, err)
		}
	}
}

// checkAgent 处理单个 agent：限流 → 下发 → 解析 → 比对/host 级错误扇出。
func checkAgent(kt *kit.Kit, bizID uint32, agentID string, exp []ExpectedProc,
	runner ScriptRunner, limiter *rate.Limiter, checkedAt time.Time) []CheckResult {

	if limiter != nil {
		if err := limiter.Wait(kt.Ctx); err != nil {
			logs.Errorf("[checkManagedProcess] biz %d agent %s rate limiter wait failed: %v", bizID, agentID, err)
			return nil
		}
	}

	osType := exp[0].OsType
	screen, err := runner.RunProcScript(kt.Ctx, agentID, osType)
	if err != nil {
		// 获取失败（任务创建/轮询失败）按解析失败处理，扇出到该 agent 全部实例。
		logs.Errorf("[checkManagedProcess] biz %d agent %s run .proc script failed: %v", bizID, agentID, err)
		return HostError(exp, table.ProcessExceptionParsingFailed,
			"进程信息获取失败，检查Agent是否正常", suggestionParsingFailed, checkedAt)
	}
	logs.Infof("[checkManagedProcess] biz %d agent %s run .proc script success: %s", bizID, agentID, screen)

	actual, perr := ParseProcScreen(screen, bizID)
	logs.Infof("[checkManagedProcess] biz %d agent %s parse .proc screen success: %v", bizID, agentID, actual)
	if perr != nil {
		errType := table.ProcessExceptionParsingFailed
		suggestion := suggestionParsingFailed
		if errors.Is(perr, ErrAgentException) {
			errType = table.ProcessExceptionAgentException
			suggestion = suggestionAgentException
		}
		return HostError(exp, errType, "进程信息解析失败: "+perr.Error(), suggestion, checkedAt)
	}

	return CompareHost(exp, actual, checkedAt)
}
