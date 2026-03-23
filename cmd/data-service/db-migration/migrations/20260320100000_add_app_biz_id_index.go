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
		Version: "20260320100000",
		Name:    "20260320100000_add_app_biz_id_index",
		Mode:    migrator.GormMode,
		Up:      mig20260320100000Up,
		Down:    mig20260320100000Down,
	})
}

// mig20260320100000Up adds an index on biz_id for the applications table.
// After multi-tenant migration, the unique index changed from (biz_id, name) to (tenant_id, biz_id, name).
// Queries that look up by biz_id alone (e.g. GetOneAppByBiz for tenant_id reverse lookup,
// ListBizTenantMap, CheckBizExists) can no longer use the leftmost prefix of the unique index.
func mig20260320100000Up(tx *gorm.DB) error {
	return tx.Exec("ALTER TABLE applications ADD INDEX idx_bizID (biz_id)").Error
}

func mig20260320100000Down(tx *gorm.DB) error {
	return tx.Exec("ALTER TABLE applications DROP INDEX idx_bizID").Error
}
