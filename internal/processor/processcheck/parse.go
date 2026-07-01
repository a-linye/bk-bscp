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

// Package processcheck 进程托管配置定时检查核心逻辑：解析 GSE .proc、构造期望项、比对分类、写异常/恢复决策。
package processcheck

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
)

// ErrAgentException 表示 Screen 中含 "agent not available" 类信号，归入 AGENT_EXCEPTION。
var ErrAgentException = errors.New("agent not available")

// ErrParsing 表示 Screen 为空 / 无法抽取 JSON / 反序列化失败，归入 PARSING_FAILED。
var ErrParsing = errors.New("parse .proc screen failed")

// ActualProc 解析自 agent .proc Screen（驼峰命名 JSON）的单个托管项。
// 仅保留参与本期判定的字段：匹配键 valuekey/contact + 9 个比对字段；
// versionCmd/healthCmd 及 GSE agent 内部字段（type/cpulmt/... ）不进入比对，反序列化时直接忽略。
type ActualProc struct {
	Contact    string `json:"contact"`
	ValueKey   string `json:"valuekey"`
	ProcName   string `json:"procName"`
	SetupPath  string `json:"setupPath"`
	PidPath    string `json:"pidPath"`
	User       string `json:"user"`
	StartCmd   string `json:"startCmd"`
	StopCmd    string `json:"stopCmd"`
	RestartCmd string `json:"restartCmd"`
	ReloadCmd  string `json:"reloadCmd"`
	KillCmd    string `json:"killCmd"`
}

// procEnvelope 对标 gsekit _parse_ip_logs：.proc 顶层为 {"proc":[{...}]}。
type procEnvelope struct {
	Proc []ActualProc `json:"proc"`
}

// jsonObjectRe 从 Screen 中抽取首个 JSON 对象（DOTALL，贪婪到最后一个 }），对标 gsekit re.search(r"\{.*\}", ...)。
var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

// ParseProcScreen 解析某 agent 的 .proc Screen，仅保留本业务（contact == GSEKIT_BIZ_{bizID}）托管项。
// 返回错误时按业务约定区分：ErrAgentException → AGENT_EXCEPTION；ErrParsing → PARSING_FAILED。
// 解析成功但本业务无托管项时返回空切片与 nil（不是错误）。
func ParseProcScreen(screen string, bizID uint32) ([]ActualProc, error) {
	// agent 异常信号优先判定：agent 不可用时通常拿不到合法 JSON，避免误判为解析失败。
	if strings.Contains(strings.ToLower(screen), "agent not available") {
		return nil, ErrAgentException
	}

	jsonStr := jsonObjectRe.FindString(screen)
	if jsonStr == "" {
		return nil, ErrParsing
	}

	var env procEnvelope
	if err := json.Unmarshal([]byte(jsonStr), &env); err != nil {
		return nil, ErrParsing
	}

	contact := gse.BuildNamespace(bizID)
	result := make([]ActualProc, 0, len(env.Proc))
	for _, p := range env.Proc {
		if p.Contact != contact {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}
