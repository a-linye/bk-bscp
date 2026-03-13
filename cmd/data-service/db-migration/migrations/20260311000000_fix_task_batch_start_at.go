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
		Version: "20260311000000",
		Name:    "20260311000000_fix_task_batch_start_at",
		Mode:    migrator.GormMode,
		Up:      mig20260311000000Up,
		Down:    mig20260311000000Down,
	})
}

// mig20260311000000Up removes the implicit ON UPDATE CURRENT_TIMESTAMP from start_at.
// MySQL automatically adds ON UPDATE CURRENT_TIMESTAMP to the first TIMESTAMP NOT NULL
// column when explicit_defaults_for_timestamp is OFF, causing start_at to be overwritten
// on every UPDATE to the row.
func mig20260311000000Up(tx *gorm.DB) error {
	return tx.Exec(
		"ALTER TABLE task_batches MODIFY COLUMN start_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '任务开始时间'",
	).Error
}

func mig20260311000000Down(tx *gorm.DB) error {
	return tx.Exec(
		"ALTER TABLE task_batches MODIFY COLUMN start_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '任务开始时间'",
	).Error
}
