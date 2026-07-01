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
	"encoding/json"
	"fmt"

	processorgse "github.com/TencentBlueKing/bk-bscp/internal/processor/gse"

	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
)

// ExpectedProc bscp DB 侧期望托管项。比对字段固定 9 个（procName 来源 Process.Spec.FuncName，
// 其余 8 字段由 ProcessInfo 经 BuildProcessOperate 渲染），不构造 versionCmd/healthCmd 及 GSE 内部字段。
type ExpectedProc struct {
	ValueKey      string                     // GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}（用别名 alias）
	ManagedStatus table.ProcessManagedStatus // 是否应托管判定基准
	// 9 个比对字段
	ProcName   string
	SetupPath  string
	PidPath    string
	User       string
	StartCmd   string
	StopCmd    string
	RestartCmd string
	ReloadCmd  string
	KillCmd    string
	// 定位与下发目标（不参与属性比对）
	ProcessInstanceID uint32
	ProcessID         uint32
	HostID            uint32
	BizID             uint32
	TenantID          string
	AgentID           string
	OsType            string
}

// BuildExpectedProcs 由单个 Process 及其实例构造期望托管项列表。
// source_data 反序列化失败 → 跳过该进程全部实例；单实例渲染失败 → 仅跳过该实例（对标 buildBizOperateItems 容错）。
func BuildExpectedProcs(process *table.Process, insts []*table.ProcessInstance, bizID uint32) []ExpectedProc {
	if process == nil || process.Spec == nil || process.Attachment == nil {
		return nil
	}

	var processInfo table.ProcessInfo
	if err := json.Unmarshal([]byte(process.Spec.SourceData), &processInfo); err != nil {
		logs.Errorf("biz %d: unmarshal process source data failed, processID=%d, err=%v", bizID, process.ID, err)
		return nil
	}

	result := make([]ExpectedProc, 0, len(insts))
	for _, inst := range insts {
		if inst == nil || inst.Spec == nil {
			continue
		}
		operate, err := processorgse.BuildProcessOperate(processorgse.BuildProcessOperateParams{
			BizID:         bizID,
			Alias:         process.Spec.Alias,
			FuncName:      process.Spec.FuncName,
			HostInstSeq:   inst.Spec.HostInstSeq,
			ModuleInstSeq: inst.Spec.ModuleInstSeq,
			SetName:       process.Spec.SetName,
			ModuleName:    process.Spec.ModuleName,
			GseOpType:     gse.OpTypeQuery,
			ProcessInfo:   processInfo,
		})
		if err != nil {
			logs.Errorf("biz %d: build expected proc failed, processID=%d, instID=%d, err=%v",
				bizID, process.ID, inst.ID, err)
			continue
		}

		valueKey := fmt.Sprintf("%s:%s", gse.BuildNamespace(bizID),
			gse.BuildProcessName(process.Spec.Alias, inst.Spec.HostInstSeq))

		result = append(result, ExpectedProc{
			ValueKey:          valueKey,
			ManagedStatus:     inst.Spec.ManagedStatus,
			ProcName:          operate.Spec.Identity.ProcName,
			SetupPath:         operate.Spec.Identity.SetupPath,
			PidPath:           operate.Spec.Identity.PidPath,
			User:              operate.Spec.Identity.User,
			StartCmd:          operate.Spec.Control.StartCmd,
			StopCmd:           operate.Spec.Control.StopCmd,
			RestartCmd:        operate.Spec.Control.RestartCmd,
			ReloadCmd:         operate.Spec.Control.ReloadCmd,
			KillCmd:           operate.Spec.Control.KillCmd,
			ProcessInstanceID: inst.ID,
			ProcessID:         process.ID,
			HostID:            process.Attachment.HostID,
			BizID:             bizID,
			TenantID:          process.Attachment.TenantID,
			AgentID:           process.Attachment.AgentID,
			OsType:            process.Spec.OsType,
		})
	}
	return result
}
