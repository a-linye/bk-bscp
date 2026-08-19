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
	"fmt"

	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/cmd/data-service/db-migration/migrator"
)

func init() {
	// add current migration to migrator
	migrator.GetMigrator().AddMigration(&migrator.Migration{
		Version: "20260817171701",
		Name:    "20260817171701_add_dimension_to_audits",
		Mode:    migrator.GormMode,
		Up:      mig20260817171701Up,
		Down:    mig20260817171701Down,
	})
}

// mig20260817171701Up for up migration
// 加列已在 20260817154117_add_core_table_columns 统一完成，本迁移只负责回填与索引切换
func mig20260817171701Up(tx *gorm.DB) error {
	tblName := "audits"

	// 1. 分批游标回填 + 分批扫尾 (针对大表采用 200/批)
	standaloneDB, err := getStandaloneDB(tx)
	if err != nil {
		return fmt.Errorf("get standalone db failed: %w", err)
	}

	if err := backfillProjectScopeTable(standaloneDB, tblName); err != nil {
		return fmt.Errorf("backfill audits failed: %w", err)
	}
	if err := sweepProjectScopeTable(standaloneDB, tblName); err != nil {
		return fmt.Errorf("sweep audits failed: %w", err)
	}

	// 2. 普通二级索引升级
	newIdxName := "idx_tenantID_bizID_projectID_appID_createdAt"
	oldIdxName := "idx_tenantID_bizID_appID_createdAt"

	if !tx.Migrator().HasIndex(tblName, newIdxName) {
		if err := execDDLWithFallback(tx,
			fmt.Sprintf("CREATE INDEX %s ON `audits` (tenant_id, biz_id, project_id, app_id, created_at) ALGORITHM=INPLACE LOCK=NONE", newIdxName),
			fmt.Sprintf("CREATE INDEX %s ON `audits` (tenant_id, biz_id, project_id, app_id, created_at)", newIdxName),
		); err != nil {
			return fmt.Errorf("create new index on audits failed: %w", err)
		}
	}

	if tx.Migrator().HasIndex(tblName, oldIdxName) {
		if err := execDDLWithFallback(tx,
			fmt.Sprintf("ALTER TABLE `audits` DROP INDEX %s, ALGORITHM=INPLACE, LOCK=NONE", oldIdxName),
			fmt.Sprintf("ALTER TABLE `audits` DROP INDEX %s", oldIdxName),
		); err != nil {
			return fmt.Errorf("drop old index on audits failed: %w", err)
		}
	}

	return nil
}

// mig20260817171701Down for down migration
func mig20260817171701Down(tx *gorm.DB) error {
	tblName := "audits"

	// 恢复旧普通索引
	if !tx.Migrator().HasIndex(tblName, "idx_tenantID_bizID_appID_createdAt") {
		if err := tx.Exec("CREATE INDEX idx_tenantID_bizID_appID_createdAt ON audits (tenant_id, biz_id, app_id, created_at)").Error; err != nil {
			return fmt.Errorf("restore old index on audits failed: %w", err)
		}
	}

	// 删新索引
	if tx.Migrator().HasIndex(tblName, "idx_tenantID_bizID_projectID_appID_createdAt") {
		if err := tx.Migrator().DropIndex(tblName, "idx_tenantID_bizID_projectID_appID_createdAt"); err != nil {
			return fmt.Errorf("drop new index on audits failed: %w", err)
		}
	}

	// 删列统一由 20260817154117_add_core_table_columns 的 Down 完成

	// 重置任务状态，使重新 Up 时可再次执行回填
	if err := resetMigrationTask(tx, tblName); err != nil {
		return err
	}

	return nil
}
