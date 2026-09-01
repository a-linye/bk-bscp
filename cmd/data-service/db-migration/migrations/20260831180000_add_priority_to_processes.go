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
	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/cmd/data-service/db-migration/migrator"
)

func init() {
	migrator.GetMigrator().AddMigration(&migrator.Migration{
		Version: "20260831180000",
		Name:    "20260831180000_add_priority_to_processes",
		Mode:    migrator.GormMode,
		Up:      mig20260831180000Up,
		Down:    mig20260831180000Down,
	})
}

func mig20260831180000Up(tx *gorm.DB) error {
	// priority 为 CMDB 进程的启动优先级，允许为负数（负数比 0 更早启动），未配置时按 0 处理
	if !tx.Migrator().HasColumn("processes", "priority") {
		if err := tx.Exec("ALTER TABLE `processes` ADD COLUMN `priority` int NOT NULL DEFAULT 0 " +
			"COMMENT '启动优先级，来源 CMDB，bscp 侧只读'").Error; err != nil {
			return err
		}
	}

	return nil
}

func mig20260831180000Down(tx *gorm.DB) error {
	if tx.Migrator().HasColumn("processes", "priority") {
		if err := tx.Exec("ALTER TABLE `processes` DROP COLUMN `priority`").Error; err != nil {
			return err
		}
	}

	return nil
}
