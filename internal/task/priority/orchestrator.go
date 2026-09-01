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

// Package priority 提供进程操作按优先级分波次串行的编排能力。
package priority

import (
	"context"
	"sync"
	"time"

	taskTypes "github.com/Tencent/bk-bcs/bcs-common/common/task/types"

	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/internal/task"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
)

var (
	taskMgrMu sync.RWMutex
	taskMgr   *task.TaskManager
)

// SetTaskManager 注入任务管理器，供进程操作回调推进后续波次。
func SetTaskManager(m *task.TaskManager) {
	taskMgrMu.Lock()
	defer taskMgrMu.Unlock()
	taskMgr = m
}

func getTaskManager() *task.TaskManager {
	taskMgrMu.RLock()
	defer taskMgrMu.RUnlock()
	return taskMgr
}

// HandleTaskComplete 在实例任务到达终态后推进批次计数与优先级波次。
// 本波次全部成功则下发下一波；存在失败则级联阻断后续待执行任务。
func HandleTaskComplete(kt *kit.Kit, daoSet dao.Set, batchID uint32, taskID string, isSuccess bool) (bool, error) {
	if batchID == 0 {
		return false, nil
	}

	result, err := daoSet.TaskBatch().CompleteTask(kt, batchID, taskID, isSuccess)
	if err != nil {
		return false, err
	}

	mgr := getTaskManager()
	if mgr == nil {
		if result.NextWave != nil || result.Cascade != nil {
			logs.Errorf("priority orchestrator has no task manager, batchID=%d next=%v cascade=%v",
				batchID, result.NextWave != nil, result.Cascade != nil)
		}
		return result.AllCompleted, nil
	}

	if result.Cascade != nil {
		if err = FailPendingTasks(kt.Ctx, mgr, result.Cascade); err != nil {
			logs.Errorf("fail pending tasks after cascade failed, batchID=%d, err=%v", batchID, err)
		}
	}
	if result.NextWave != nil {
		if err = EnqueueWave(kt.Ctx, mgr, daoSet, kt, batchID, result.NextWave); err != nil {
			logs.Errorf("enqueue next priority wave failed, batchID=%d wave=%d, err=%v",
				batchID, result.NextWave.Seq, err)
		}
	}
	return result.AllCompleted, nil
}

// FailPendingTasks 将后续波次中尚未执行的任务置为失败
func FailPendingTasks(ctx context.Context, mgr *task.TaskManager, cascade *table.PriorityCascade) error {
	if cascade == nil || len(cascade.PendingTaskIDs) == 0 {
		return nil
	}
	msg := table.CascadeBlockMessage(cascade.FailedPriority, cascade.Order)
	var firstErr error
	for _, taskID := range cascade.PendingTaskIDs {
		if err := mgr.FailPending(ctx, taskID, msg); err != nil {
			logs.Errorf("fail pending task %s failed: %v", taskID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// EnqueueWave 下发指定波次内全部已落库任务，并标记该波次已下发
func EnqueueWave(ctx context.Context, mgr *task.TaskManager, daoSet dao.Set, kt *kit.Kit,
	batchID uint32, wave *table.PriorityWave) error {
	if wave == nil || len(wave.TaskIDs) == 0 {
		return nil
	}
	var firstErr error
	for _, taskID := range wave.TaskIDs {
		t, err := mgr.GetTaskWithID(ctx, taskID)
		if err != nil {
			logs.Errorf("get task %s for enqueue failed: %v", taskID, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		status := t.GetStatus()
		if status != taskTypes.TaskStatusInit && status != taskTypes.TaskStatusNotStarted &&
			status != taskTypes.TaskStatusFailure && status != taskTypes.TaskStatusTimeout {
			continue
		}
		if err = mgr.Enqueue(t); err != nil {
			logs.Errorf("enqueue task %s failed: %v", taskID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if daoSet != nil && kt != nil && firstErr == nil {
		if err := daoSet.TaskBatch().MarkWaveDispatched(kt, batchID, wave.Seq); err != nil {
			logs.Errorf("mark wave %d dispatched failed, batchID=%d, err=%v", wave.Seq, batchID, err)
			return err
		}
	}
	return firstErr
}

const resumeInterval = 15 * time.Second

// StartResumer 仅在 master 上周期性恢复未完成的优先级编排（服务重启后补发下一波或补齐级联）。
func StartResumer(ctx context.Context, daoSet dao.Set, mgr *task.TaskManager, isMaster func() bool) {
	if daoSet == nil || mgr == nil || isMaster == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(resumeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !isMaster() {
					continue
				}
				if err := ResumeRunningPlans(kit.New().WithSkipTenantFilter(), daoSet, mgr); err != nil {
					logs.Errorf("resume priority plans failed: %v", err)
				}
			}
		}
	}()
}

// ResumeRunningPlans 扫描运行中的进程操作批次，按持久化计划补发或补齐级联。
func ResumeRunningPlans(kt *kit.Kit, daoSet dao.Set, mgr *task.TaskManager) error {
	batches, err := daoSet.TaskBatch().ListRunningByObject(kt, table.TaskObjectProcess)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		plan, planErr := batch.Spec.GetPriorityPlan()
		if planErr != nil || plan == nil || len(plan.Waves) == 0 {
			continue
		}
		if plan.Blocked {
			idx := plan.CurrentWave
			if idx < 0 || idx >= len(plan.Waves) {
				idx = 0
			}
			pending := plan.PendingTaskIDsAfter(idx)
			if len(pending) == 0 {
				continue
			}
			_ = FailPendingTasks(kt.Ctx, mgr, &table.PriorityCascade{
				FailedPriority: plan.BlockedPriority,
				Order:          plan.Order,
				PendingTaskIDs: pending,
			})
			continue
		}
		wave := plan.Waves[plan.CurrentWave]
		if wave == nil || wave.Dispatched {
			continue
		}
		if err = EnqueueWave(kt.Ctx, mgr, daoSet, kt, batch.ID, wave); err != nil {
			logs.Errorf("resume enqueue wave failed, batchID=%d wave=%d, err=%v", batch.ID, wave.Seq, err)
		}
	}
	return nil
}
