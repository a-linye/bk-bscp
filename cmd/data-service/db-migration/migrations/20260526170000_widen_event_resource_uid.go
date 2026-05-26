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
		Version: "20260526170000",
		Name:    "20260526170000_widen_event_resource_uid",
		Mode:    migrator.GormMode,
		Up:      mig20260526170000Up,
		Down:    mig20260526170000Down,
	})
}

func mig20260526170000Up(tx *gorm.DB) error {
	if err := tx.Exec("ALTER TABLE `events` MODIFY COLUMN `resource_uid` varchar(256) DEFAULT ''").Error; err != nil {
		return err
	}
	return nil
}

func mig20260526170000Down(tx *gorm.DB) error {
	if err := tx.Exec("ALTER TABLE `events` MODIFY COLUMN `resource_uid` varchar(64) DEFAULT ''").Error; err != nil {
		return err
	}
	return nil
}
