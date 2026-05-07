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
		Version: "20260507100000",
		Name:    "20260507100000_add_released_ci_biz_release_index",
		Mode:    migrator.GormMode,
		Up:      mig20260507100000Up,
		Down:    mig20260507100000Down,
	})
}

// mig20260507100000Up adds an index on (biz_id, release_id) for the released_config_items table.
// ListAllByReleaseIDs queries by release_id IN (...) AND biz_id = ?, but the existing composite
// index idx_tenantID_bizID_appID_relID has release_id at position 4. Without app_id in the WHERE
// clause the leftmost prefix only covers (tenant_id, biz_id), making release_id filtering a scan.
func mig20260507100000Up(tx *gorm.DB) error {
	return tx.Exec("ALTER TABLE released_config_items ADD INDEX idx_tenantID_bizID_releaseID (tenant_id, biz_id, release_id)").Error
}

func mig20260507100000Down(tx *gorm.DB) error {
	return tx.Exec("ALTER TABLE released_config_items DROP INDEX idx_tenantID_bizID_releaseID").Error
}
