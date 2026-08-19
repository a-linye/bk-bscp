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
		Version: "20260817170113",
		Name:    "20260817170113_add_dimension_to_clients",
		Mode:    migrator.GormMode,
		Up:      mig20260817170113Up,
		Down:    mig20260817170113Down,
	})
}

// 发布时序约束（重要）：本迁移会删除旧唯一索引 idx_bizID_appID_uid。
// 旧版本 data-service 心跳 UPSERT 不会写 project_id/environment_id（保持 NULL），
// 而 MySQL 唯一索引中 NULL ≠ NULL，旧唯一键删除后旧实例的 UPSERT 将退化为 INSERT
// 并持续产生重复行且不自愈。因此本迁移必须在新版本 data-service 全量 ready 之后
// 单独执行（migrate up --step 1）；其余维度迁移（20260817152329/154117/161906/17171701）
// 只加可空列、回填、切小表索引或普通索引，可在新版本上线前执行。
//
// mig20260817170113Up for up migration
// 加列已在 20260817154117_add_core_table_columns 统一完成，本迁移只负责回填与索引切换
func mig20260817170113Up(tx *gorm.DB) error {
	tblName := "clients"

	// 1. 分批游标回填 + 分批扫尾 (保留旧唯一索引 idx_bizID_appID_uid)
	standaloneDB, err := getStandaloneDB(tx)
	if err != nil {
		return fmt.Errorf("get standalone db failed: %w", err)
	}

	if err := backfillEnvScopeTable(standaloneDB, tblName); err != nil {
		return fmt.Errorf("backfill clients failed: %w", err)
	}
	if err := sweepEnvScopeTable(standaloneDB, tblName); err != nil {
		return fmt.Errorf("sweep clients failed: %w", err)
	}

	// 2. 断言维度无残留：残留 NULL 行在新唯一索引下失去约束，删除旧索引前必须中止
	if err := assertScopeFilled(standaloneDB, tblName); err != nil {
		return err
	}

	// 3. 无锁创建新唯一索引
	newIdxName := "idx_bizID_projectID_envID_appID_uid"
	if !tx.Migrator().HasIndex(tblName, newIdxName) {
		if err := execDDLWithFallback(tx,
			fmt.Sprintf("CREATE UNIQUE INDEX %s ON `clients` (biz_id, project_id, environment_id, app_id, uid) ALGORITHM=INPLACE LOCK=NONE", newIdxName),
			fmt.Sprintf("CREATE UNIQUE INDEX %s ON `clients` (biz_id, project_id, environment_id, app_id, uid)", newIdxName),
		); err != nil {
			return fmt.Errorf("create new unique index on clients failed: %w", err)
		}
	}

	// 4. 确认新索引成功后，删旧唯一索引
	oldIdxName := "idx_bizID_appID_uid"
	if tx.Migrator().HasIndex(tblName, oldIdxName) {
		if err := execDDLWithFallback(tx,
			fmt.Sprintf("ALTER TABLE `clients` DROP INDEX %s, ALGORITHM=INPLACE, LOCK=NONE", oldIdxName),
			fmt.Sprintf("ALTER TABLE `clients` DROP INDEX %s", oldIdxName),
		); err != nil {
			return fmt.Errorf("drop old unique index on clients failed: %w", err)
		}
	}

	return nil
}

// mig20260817170113Down for down migration
func mig20260817170113Down(tx *gorm.DB) error {
	tblName := "clients"

	// 恢复旧唯一索引
	if !tx.Migrator().HasIndex(tblName, "idx_bizID_appID_uid") {
		if err := tx.Exec("CREATE UNIQUE INDEX idx_bizID_appID_uid ON clients (biz_id, app_id, uid)").Error; err != nil {
			return fmt.Errorf("restore idx_bizID_appID_uid failed: %w", err)
		}
	}

	// 删除新唯一索引
	if tx.Migrator().HasIndex(tblName, "idx_bizID_projectID_envID_appID_uid") {
		if err := tx.Migrator().DropIndex(tblName, "idx_bizID_projectID_envID_appID_uid"); err != nil {
			return fmt.Errorf("drop idx_bizID_projectID_envID_appID_uid failed: %w", err)
		}
	}

	// 删除列统一由 20260817154117_add_core_table_columns 的 Down 完成

	// 重置任务状态，使重新 Up 时可再次执行回填
	if err := resetMigrationTask(tx, tblName); err != nil {
		return err
	}

	return nil
}
