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
		Version: "20260507114600",
		Name:    "20260507114600_modify_process_environment_nullable",
		Mode:    migrator.GormMode,
		Up:      mig20260507114600Up,
		Down:    mig20260507114600Down,
	})
}

// mig20260507114600Up makes the environment column nullable in processes table
func mig20260507114600Up(tx *gorm.DB) error {
	return tx.Exec("ALTER TABLE processes MODIFY COLUMN environment varchar(128) DEFAULT '' COMMENT '环境类型(1:测试, 2:体验, 3:正式)'").Error
}

func mig20260507114600Down(tx *gorm.DB) error {
	// Up 将列改为可空后，线上可能已写入 NULL；直接 MODIFY NOT NULL 会失败，需先与 Up 侧 DEFAULT '' 语义对齐再收紧。
	if err := tx.Exec("UPDATE processes SET environment = '' WHERE environment IS NULL").Error; err != nil {
		return err
	}
	return tx.Exec("ALTER TABLE processes MODIFY COLUMN environment varchar(128) NOT NULL COMMENT '环境类型(1:测试, 2:体验, 3:正式)'").Error
}
