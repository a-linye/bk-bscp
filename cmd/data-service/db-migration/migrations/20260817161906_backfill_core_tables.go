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
		Version: "20260817161906",
		Name:    "20260817161906_backfill_core_tables",
		Mode:    migrator.GormMode,
		Up:      mig20260817161906Up,
		Down:    mig20260817161906Down,
	})
}

// 回填范围仅覆盖核心小表；audits / clients 为大表，由各自独立的 migration
// （20260817170113 / 20260817171701）负责，保持失败域隔离与 step 边界可用
var backfillProjectTables = []string{"hooks", "groups", "credentials", "template_spaces", "template_variables"}
var backfillEnvTables = []string{"applications", "credential_scopes", "events", "client_querys"}

// mig20260817161906Up for up migration
func mig20260817161906Up(tx *gorm.DB) error {
	standaloneDB, err := getStandaloneDB(tx)
	if err != nil {
		return fmt.Errorf("get standalone db failed: %w", err)
	}

	// 1. 分批回填（带任务状态，可断点续跑，统一租户归一化匹配）
	for _, tbl := range backfillProjectTables {
		if err := backfillProjectScopeTable(standaloneDB, tbl); err != nil {
			return fmt.Errorf("backfill %s failed: %w", tbl, err)
		}
	}
	for _, tbl := range backfillEnvTables {
		if err := backfillEnvScopeTable(standaloneDB, tbl); err != nil {
			return fmt.Errorf("backfill %s failed: %w", tbl, err)
		}
	}

	// 2. 分批扫尾（补偿回填窗口期并发写入导致的维度缺失行）
	for _, tbl := range backfillProjectTables {
		if err := sweepProjectScopeTable(standaloneDB, tbl); err != nil {
			return fmt.Errorf("sweep %s failed: %w", tbl, err)
		}
	}
	for _, tbl := range backfillEnvTables {
		if err := sweepEnvScopeTable(standaloneDB, tbl); err != nil {
			return fmt.Errorf("sweep %s failed: %w", tbl, err)
		}
	}

	// 3. 将业务级共享的内置模板空间置为 0（须在切换唯一索引前完成，
	//    残留 NULL 会使唯一索引对该行失效；若存量已重复则此处撞键显式失败）
	if err := normalizeGlobalTemplateSpaces(standaloneDB); err != nil {
		return fmt.Errorf("normalize global template spaces failed: %w", err)
	}

	// 4. 切换唯一索引（先无锁建新索引 -> 再删旧索引，切换前断言无残留）
	if err := stepSwitchCoreIndexes(standaloneDB); err != nil {
		return fmt.Errorf("switch core table indexes failed: %w", err)
	}

	return nil
}

// mig20260817161906Down for down migration
func mig20260817161906Down(tx *gorm.DB) error {
	// 逆序回滚：恢复旧索引，清理新索引，并重置任务状态使重新 Up 时可再次回填
	if err := stepRestoreCoreIndexes(tx); err != nil {
		return err
	}
	for _, tbl := range append(append([]string{}, backfillProjectTables...), backfillEnvTables...) {
		if err := resetMigrationTask(tx, tbl); err != nil {
			return err
		}
	}
	return nil
}

type indexAdj struct {
	table    string
	oldIdx   string
	newIdx   string
	newCols  string
	isUnique bool
}

var coreIndexAdjustments = []indexAdj{
	{"groups", "idx_tenantID_bizID_name", "idx_tenantID_bizID_projectID_name", "tenant_id, biz_id, project_id, name", true},
	{"hooks", "idx_tenantID_bizID_name", "idx_tenantID_bizID_projectID_name", "tenant_id, biz_id, project_id, name", true},
	{"credentials", "idx_tenantID_bizID_name", "idx_tenantID_bizID_projectID_name", "tenant_id, biz_id, project_id, name", true},
	{"template_spaces", "idx_tenantID_bizID_name", "idx_tenantID_bizID_projectID_name", "tenant_id, biz_id, project_id, name", true},
	{"template_variables", "idx_tenantID_bizID_name", "idx_tenantID_bizID_projectID_name", "tenant_id, biz_id, project_id, name", true},
	{"client_querys", "idx_tenantID_bizID_appID_creator",
		"idx_tenantID_bizID_projectID_envID_appID_creator",
		"tenant_id, biz_id, project_id, environment_id, app_id, creator", false},
	{"events", "idx_tenantID_resource_bizID", "idx_tenantID_resource_bizID_projectID_envID", "tenant_id, resource, biz_id, project_id, environment_id", false},
	{"applications", "idx_tenantID_bizID_name", "idx_tenantID_bizID_projectID_envID_name", "tenant_id, biz_id, project_id, environment_id, name", true},
}

func stepRestoreCoreIndexes(db *gorm.DB) error {
	for _, adj := range coreIndexAdjustments {
		if !db.Migrator().HasIndex(adj.table, adj.oldIdx) {
			idxType := ""
			if adj.isUnique {
				idxType = "UNIQUE "
			}
			oldCols := "tenant_id, biz_id, name"
			switch adj.table {
			case "client_querys":
				oldCols = "tenant_id, biz_id, app_id, creator"
			case "events":
				oldCols = "tenant_id, resource, biz_id"
			}
			sql := fmt.Sprintf("CREATE %sINDEX %s ON `%s` (%s)", idxType, adj.oldIdx, adj.table, oldCols)
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("restore old index %s on %s failed: %w", adj.oldIdx, adj.table, err)
			}
		}

		if db.Migrator().HasIndex(adj.table, adj.newIdx) {
			if err := db.Migrator().DropIndex(adj.table, adj.newIdx); err != nil {
				return fmt.Errorf("drop new index %s on %s failed: %w", adj.newIdx, adj.table, err)
			}
		}
	}

	return nil
}

func stepSwitchCoreIndexes(db *gorm.DB) error {
	// 断言所有待切换唯一索引的表维度无残留：残留 NULL 行在新唯一索引下失去约束，
	// 删除旧索引后不会自愈，必须在切换前中止（events/client_querys 为普通索引无需断言）
	for _, adj := range coreIndexAdjustments {
		if !adj.isUnique {
			continue
		}
		if !db.Migrator().HasIndex(adj.table, adj.oldIdx) {
			continue
		}
		if err := assertScopeFilled(db, adj.table); err != nil {
			return err
		}
	}

	for _, adj := range coreIndexAdjustments {
		if !db.Migrator().HasIndex(adj.table, adj.newIdx) {
			idxType := ""
			if adj.isUnique {
				idxType = "UNIQUE "
			}
			if err := execDDLWithFallback(db,
				fmt.Sprintf("CREATE %sINDEX %s ON `%s` (%s) ALGORITHM=INPLACE LOCK=NONE", idxType, adj.newIdx, adj.table, adj.newCols),
				fmt.Sprintf("CREATE %sINDEX %s ON `%s` (%s)", idxType, adj.newIdx, adj.table, adj.newCols),
			); err != nil {
				return fmt.Errorf("create new index %s on %s failed: %w", adj.newIdx, adj.table, err)
			}
		}
	}

	for _, adj := range coreIndexAdjustments {
		if db.Migrator().HasIndex(adj.table, adj.oldIdx) {
			if err := execDDLWithFallback(db,
				fmt.Sprintf("ALTER TABLE `%s` DROP INDEX %s, ALGORITHM=INPLACE, LOCK=NONE", adj.table, adj.oldIdx),
				fmt.Sprintf("ALTER TABLE `%s` DROP INDEX %s", adj.table, adj.oldIdx),
			); err != nil {
				return fmt.Errorf("drop old index %s on %s failed: %w", adj.oldIdx, adj.table, err)
			}
		}
	}

	return nil
}
