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
		Version: "20260817154117",
		Name:    "20260817154117_add_core_table_columns",
		Mode:    migrator.GormMode,
		Up:      mig20260817154117Up,
		Down:    mig20260817154117Down,
	})
}

// 统一在此为全部维度表加列（ALGORITHM=INSTANT 纯元数据操作，与表大小无关），
// 大表 clients/audits 的回填与索引切换仍由各自独立的 migration 完成。
var coreProjectTables = []string{"hooks", "groups", "credentials", "template_spaces", "template_variables", "audits"}
var coreEnvTables = []string{"applications", "credential_scopes", "events", "client_querys", "clients"}

// mig20260817154117Up for up migration
func mig20260817154117Up(tx *gorm.DB) error {
	// project_id / environment_id 均保持 NULL。
	// 本 migration 只负责 DDL，不进行数据回填，不修改现有索引。
	for _, tbl := range coreProjectTables {
		if !tx.Migrator().HasColumn(tbl, "project_id") {
			if err := execDDLWithFallback(tx,
				fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `project_id` bigint(20) unsigned DEFAULT NULL, ALGORITHM=INSTANT", tbl),
				fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `project_id` bigint(20) unsigned DEFAULT NULL", tbl),
			); err != nil {
				return fmt.Errorf("add column project_id to table %s failed: %w", tbl, err)
			}
		}
	}

	for _, tbl := range coreEnvTables {
		if !tx.Migrator().HasColumn(tbl, "project_id") {
			if err := execDDLWithFallback(tx,
				fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `project_id` bigint(20) unsigned DEFAULT NULL, ALGORITHM=INSTANT", tbl),
				fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `project_id` bigint(20) unsigned DEFAULT NULL", tbl),
			); err != nil {
				return fmt.Errorf("add column project_id to table %s failed: %w", tbl, err)
			}
		}
		if !tx.Migrator().HasColumn(tbl, "environment_id") {
			if err := execDDLWithFallback(tx,
				fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `environment_id` bigint(20) unsigned DEFAULT NULL, ALGORITHM=INSTANT", tbl),
				fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `environment_id` bigint(20) unsigned DEFAULT NULL", tbl),
			); err != nil {
				return fmt.Errorf("add column environment_id to table %s failed: %w", tbl, err)
			}
		}
		if tbl == "applications" && !tx.Migrator().HasColumn(tbl, "env_display") {
			if err := execDDLWithFallback(tx,
				"ALTER TABLE `applications` ADD COLUMN `env_display` varchar(280) NULL, ALGORITHM=INSTANT",
				"ALTER TABLE `applications` ADD COLUMN `env_display` varchar(280) NULL",
			); err != nil {
				return fmt.Errorf("add column env_display to table %s failed: %w", tbl, err)
			}
		}
	}

	return nil
}

// mig20260817154117Down for down migration
func mig20260817154117Down(tx *gorm.DB) error {
	// 仅删列
	for _, tbl := range coreProjectTables {
		if tx.Migrator().HasColumn(tbl, "project_id") {
			if err := tx.Migrator().DropColumn(tbl, "project_id"); err != nil {
				return fmt.Errorf("drop column project_id from table %s failed: %w", tbl, err)
			}
		}
	}

	for _, tbl := range coreEnvTables {
		if tx.Migrator().HasColumn(tbl, "project_id") {
			if err := tx.Migrator().DropColumn(tbl, "project_id"); err != nil {
				return fmt.Errorf("drop column project_id from table %s failed: %w", tbl, err)
			}
		}

		if tx.Migrator().HasColumn(tbl, "environment_id") {
			if err := tx.Migrator().DropColumn(tbl, "environment_id"); err != nil {
				return fmt.Errorf("drop column environment_id from table %s failed: %w", tbl, err)
			}
		}

		if tbl == "applications" && tx.Migrator().HasColumn(tbl, "env_display") {
			if err := tx.Migrator().DropColumn(tbl, "env_display"); err != nil {
				return fmt.Errorf("drop column env_display from table %s failed: %w", tbl, err)
			}
		}
	}

	return nil
}
