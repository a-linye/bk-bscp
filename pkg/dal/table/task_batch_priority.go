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

package table

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PriorityOrder 优先级波次的排序方向
type PriorityOrder string

const (
	// PriorityOrderAsc 升序：优先级小的先执行（start / reload / kill）
	PriorityOrderAsc PriorityOrder = "asc"
	// PriorityOrderDesc 降序：优先级大的先执行（stop / restart）
	PriorityOrderDesc PriorityOrder = "desc"
)

// CascadeBlockMessage 生成对齐 gsekit 的优先级级联阻断文案
func CascadeBlockMessage(failedPriority int, order PriorityOrder) string {
	if order == PriorityOrderDesc {
		return fmt.Sprintf("优先级等于[%d]的进程操作已失败，优先级小于此的进程操作不会被继续执行", failedPriority)
	}
	return fmt.Sprintf("优先级等于[%d]的进程操作已失败，优先级大于此的进程操作不会被继续执行", failedPriority)
}

// TaskBatchExtraData 任务批次的扩展数据，序列化后存放于 task_batches.extra_data
type TaskBatchExtraData struct {
	// GroupID 进程操作在任务框架中对应的任务组 ID，编排进度以任务组为准
	GroupID string `json:"group_id,omitempty"`
	// RegisterProcess 更新托管任务的成功计数，与优先级编排共用 extra_data
	RegisterProcess *RegisterProcessExtra `json:"register_process,omitempty"`
}

// RegisterProcessExtra 更新托管扩展参数
type RegisterProcessExtra struct {
	SuccessCount uint32 `json:"success_count"`
}

// GetExtraData 解析批次扩展数据，extra_data 为空时返回空结构
func (t *TaskBatchSpec) GetExtraData() (*TaskBatchExtraData, error) {
	extra := &TaskBatchExtraData{}
	if t.ExtraData == "" || t.ExtraData == "{}" {
		return extra, nil
	}
	if err := json.Unmarshal([]byte(t.ExtraData), extra); err != nil {
		return nil, err
	}
	return extra, nil
}

// SetExtraData 序列化批次扩展数据
func (t *TaskBatchSpec) SetExtraData(extra *TaskBatchExtraData) error {
	if extra == nil {
		return errors.New("extra data not set")
	}
	b, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	t.ExtraData = string(b)
	return nil
}
