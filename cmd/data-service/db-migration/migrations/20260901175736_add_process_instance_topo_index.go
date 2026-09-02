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
	// add current migration to migrator
	migrator.GetMigrator().AddMigration(&migrator.Migration{
		Version: "20260901175736",
		Name:    "20260901175736_add_process_instance_topo_index",
		Mode:    migrator.GormMode,
		Up:      mig20260901175736Up,
		Down:    mig20260901175736Down,
	})
}

// mig20260901175736Up for up migration
func mig20260901175736Up(tx *gorm.DB) error {
	var count int64
	if err := tx.Raw(
		`SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		AND table_name = 'process_instances'
		AND index_name = 'idx_processID_status_managedStatus'`,
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	return tx.Exec(
		"ALTER TABLE process_instances ADD INDEX idx_processID_status_managedStatus (process_id, status, managed_status)",
	).Error
}

// mig20260901175736Down for down migration
func mig20260901175736Down(tx *gorm.DB) error {
	// The index may have existed before this migration (for example, it may
	// have been created manually during the incident). Since this migration
	// cannot distinguish ownership of an existing index, do not drop it on
	// rollback.

	return nil
}
