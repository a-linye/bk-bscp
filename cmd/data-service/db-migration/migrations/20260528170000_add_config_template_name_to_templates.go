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
		Version: "20260528170000",
		Name:    "20260528170000_add_config_template_name_to_templates",
		Mode:    migrator.GormMode,
		Up:      mig20260528170000Up,
		Down:    mig20260528170000Down,
	})
}

func mig20260528170000Up(tx *gorm.DB) error {
	// 1. Add config_template_name column
	if !tx.Migrator().HasColumn("templates", "config_template_name") {
		if err := tx.Exec("ALTER TABLE `templates` ADD COLUMN `config_template_name` varchar(255) NOT NULL DEFAULT ''").Error; err != nil {
			return err
		}
	}

	// 2. Drop old unique index
	if tx.Migrator().HasIndex("templates", "idx_tenantID_bizID_tempSpaID_name_path") {
		if err := tx.Exec("DROP INDEX `idx_tenantID_bizID_tempSpaID_name_path` ON `templates`").Error; err != nil {
			return err
		}
	}

	// 3. Create new unique index including config_template_name
	if !tx.Migrator().HasIndex("templates", "idx_tenantID_bizID_tempSpaID_name_path_ctName") {
		if err := tx.Exec("CREATE UNIQUE INDEX `idx_tenantID_bizID_tempSpaID_name_path_ctName` " +
			"ON `templates` (`tenant_id`, `biz_id`, `template_space_id`, `name`(100), `path`(100), " +
			"`config_template_name`(100))").Error; err != nil {
			return err
		}
	}

	return nil
}

func mig20260528170000Down(tx *gorm.DB) error {
	// 1. Drop new unique index
	if tx.Migrator().HasIndex("templates", "idx_tenantID_bizID_tempSpaID_name_path_ctName") {
		if err := tx.Exec("DROP INDEX `idx_tenantID_bizID_tempSpaID_name_path_ctName` ON `templates`").Error; err != nil {
			return err
		}
	}

	// 2. Restore old unique index
	if !tx.Migrator().HasIndex("templates", "idx_tenantID_bizID_tempSpaID_name_path") {
		if err := tx.Exec("CREATE UNIQUE INDEX `idx_tenantID_bizID_tempSpaID_name_path` " +
			"ON `templates` (`tenant_id`, `biz_id`, `template_space_id`, `name`(100), `path`(100))").Error; err != nil {
			return err
		}
	}

	// 3. Drop config_template_name column
	if tx.Migrator().HasColumn("templates", "config_template_name") {
		if err := tx.Exec("ALTER TABLE `templates` DROP COLUMN `config_template_name`").Error; err != nil {
			return err
		}
	}

	return nil
}
