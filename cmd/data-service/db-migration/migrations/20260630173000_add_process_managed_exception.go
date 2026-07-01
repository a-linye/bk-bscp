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

package migrations

import (
	"time"

	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/cmd/data-service/db-migration/migrator"
)

func init() {
	// add current migration to migrator
	migrator.GetMigrator().AddMigration(&migrator.Migration{
		Version: "20260630173000",
		Name:    "20260630173000_add_process_managed_exception",
		Mode:    migrator.GormMode,
		Up:      mig20260630173000Up,
		Down:    mig20260630173000Down,
	})
}

// nolint
// mig20260630173000Up for up migration
func mig20260630173000Up(tx *gorm.DB) error {
	// ProcessManagedExceptions 进程托管异常记录表
	type ProcessManagedExceptions struct {
		ID                uint   `gorm:"type:bigint(1) unsigned not null;primaryKey;autoIncrement:false"`
		TenantID          string `gorm:"column:tenant_id;type:varchar(255);not null;default:default;comment:租户ID" json:"tenant_id"`
		BizID             uint   `gorm:"column:biz_id;type:bigint unsigned;not null;index:idx_bizID_processInstanceID,priority:1;comment:业务ID" json:"biz_id"`                                             // 业务ID
		HostID            uint   `gorm:"column:host_id;type:bigint unsigned;not null;comment:主机ID" json:"host_id"`                                                                                        // 主机ID
		ProcessID         uint   `gorm:"column:process_id;type:bigint unsigned;not null;comment:关联的process表ID" json:"process_id"`                                                                         // 关联的process表ID
		ProcessInstanceID uint   `gorm:"column:process_instance_id;type:bigint unsigned;not null;index:idx_bizID_processInstanceID,priority:2;comment:关联的process_instance表ID" json:"process_instance_id"` // 关联的process_instance表ID

		ErrorType          string    `gorm:"column:error_type;type:varchar(64);not null;comment:异常类型" json:"error_type"`                                // 异常类型
		ErrorMsg           string    `gorm:"column:error_msg;type:text;comment:异常描述" json:"error_msg"`                                                  // 异常描述
		HandlingSuggestion string    `gorm:"column:handling_suggestion;type:varchar(1024);not null;default:'';comment:处理建议" json:"handling_suggestion"` // 处理建议
		Status             string    `gorm:"column:status;type:varchar(32);not null;default:exception;comment:记录状态:exception,recovered" json:"status"`  // 记录状态:exception,recovered
		CheckedAt          time.Time `gorm:"column:checked_at;type:datetime;not null;comment:检查时间" json:"checked_at"`                                   // 检查时间

		// Revision is revision info of the resource
		Creator   string    `gorm:"type:varchar(64) not null" json:"creator"`
		Reviser   string    `gorm:"type:varchar(64) not null" json:"reviser"`
		CreatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
		UpdatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`
	}

	// IDGenerators : ID生成器
	type IDGenerators struct {
		ID        uint      `gorm:"type:bigint(1) unsigned not null;primaryKey"`
		Resource  string    `gorm:"type:varchar(50) not null;uniqueIndex:idx_resource"`
		MaxID     uint      `gorm:"type:bigint(1) unsigned not null"`
		UpdatedAt time.Time `gorm:"type:datetime(6) not null"`
	}

	if err := tx.Set("gorm:table_options", "ENGINE=InnoDB CHARSET=utf8mb4").
		AutoMigrate(&ProcessManagedExceptions{}); err != nil {
		return err
	}

	now := time.Now()
	if result := tx.Create([]IDGenerators{
		{Resource: "process_managed_exceptions", MaxID: 0, UpdatedAt: now},
	}); result.Error != nil {
		return result.Error
	}

	return nil
}

// mig20260630173000Down for down migration
func mig20260630173000Down(tx *gorm.DB) error {
	// IDGenerators : ID生成器
	type IDGenerators struct {
		ID        uint      `gorm:"type:bigint(1) unsigned not null;primaryKey"`
		Resource  string    `gorm:"type:varchar(50) not null;uniqueIndex:idx_resource"`
		MaxID     uint      `gorm:"type:bigint(1) unsigned not null"`
		UpdatedAt time.Time `gorm:"type:datetime(6) not null"`
	}

	var resources = []string{
		"process_managed_exceptions",
	}
	if result := tx.Where("resource IN ?", resources).Delete(&IDGenerators{}); result.Error != nil {
		return result.Error
	}

	if err := tx.Migrator().DropTable("process_managed_exceptions"); err != nil {
		return err
	}

	return nil
}
