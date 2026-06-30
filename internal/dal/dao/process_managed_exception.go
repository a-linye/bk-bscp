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

package dao

import (
	"errors"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/dal/gen"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// ProcessManagedException 进程托管异常记录 DAO 接口。
// 由后台巡检自动写入/恢复，非用户资源操作，不接审计。
type ProcessManagedException interface {
	// Create 追加写入一条异常记录，ID 由 idGen 分配；非覆盖。
	Create(kit *kit.Kit, m *table.ProcessManagedException) (uint32, error)
	// ListByProcessInstanceID 返回该进程实例的全部历史记录（按 id 倒序）。
	ListByProcessInstanceID(kit *kit.Kit, bizID, processInstanceID uint32) ([]*table.ProcessManagedException, error)
	// GetLatestByProcessInstanceID 取该进程实例最新一条记录；无记录返回 ErrRecordNotFound。
	GetLatestByProcessInstanceID(kit *kit.Kit, bizID, processInstanceID uint32) (*table.ProcessManagedException, error)
	// IsException 以最新一条记录状态判定进程实例当前是否异常；无记录返回 false。
	IsException(kit *kit.Kit, bizID, processInstanceID uint32) (bool, error)
	// UpdateStatus 将目标记录状态更新（恢复语义），刷新 reviser/updated_at；历史明细保留。
	UpdateStatus(kit *kit.Kit, bizID, id uint32, status table.ProcessExceptionStatus) error
}

var _ ProcessManagedException = new(processManagedExceptionDao)

type processManagedExceptionDao struct {
	genQ     *gen.Query
	idGen    IDGenInterface
	auditDao AuditDao
}

// Create implements ProcessManagedException.
func (dao *processManagedExceptionDao) Create(kit *kit.Kit, m *table.ProcessManagedException) (uint32, error) {
	if m == nil {
		return 0, errors.New("process managed exception is nil")
	}

	id, err := dao.idGen.One(kit, table.ProcessManagedExceptionsTable)
	if err != nil {
		return 0, err
	}
	m.ID = id

	// 写库失败直接返回 error，由调用方决定是否阻断，不在 DAO 层重试/吞错。
	if err := dao.genQ.ProcessManagedException.WithContext(kit.Ctx).Create(m); err != nil {
		return 0, err
	}

	return id, nil
}

// ListByProcessInstanceID implements ProcessManagedException.
func (dao *processManagedExceptionDao) ListByProcessInstanceID(kit *kit.Kit, bizID, processInstanceID uint32) (
	[]*table.ProcessManagedException, error) {
	m := dao.genQ.ProcessManagedException

	// 租户隔离由 set_tenant_id 回调自动追加 tenant_id 条件。
	return dao.genQ.ProcessManagedException.WithContext(kit.Ctx).
		Where(m.BizID.Eq(bizID), m.ProcessInstanceID.Eq(processInstanceID)).
		Order(m.ID.Desc()).
		Find()
}

// GetLatestByProcessInstanceID implements ProcessManagedException.
func (dao *processManagedExceptionDao) GetLatestByProcessInstanceID(kit *kit.Kit, bizID, processInstanceID uint32) (
	*table.ProcessManagedException, error) {
	m := dao.genQ.ProcessManagedException

	return dao.genQ.ProcessManagedException.WithContext(kit.Ctx).
		Where(m.BizID.Eq(bizID), m.ProcessInstanceID.Eq(processInstanceID)).
		Order(m.ID.Desc()).
		Take()
}

// IsException implements ProcessManagedException.
func (dao *processManagedExceptionDao) IsException(kit *kit.Kit, bizID, processInstanceID uint32) (bool, error) {
	latest, err := dao.GetLatestByProcessInstanceID(kit, bizID, processInstanceID)
	if err != nil {
		// 无记录视为不异常；其他错误透传。
		if errors.Is(err, ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	return latest.Spec.Status == table.ProcessExceptionStatusException, nil
}

// UpdateStatus implements ProcessManagedException.
func (dao *processManagedExceptionDao) UpdateStatus(kit *kit.Kit, bizID, id uint32,
	status table.ProcessExceptionStatus) error {
	m := dao.genQ.ProcessManagedException

	_, err := dao.genQ.ProcessManagedException.WithContext(kit.Ctx).
		Where(m.BizID.Eq(bizID), m.ID.Eq(id)).
		Updates(map[string]any{
			"status":     status,
			"reviser":    kit.User,
			"updated_at": time.Now(),
		})
	return err
}
