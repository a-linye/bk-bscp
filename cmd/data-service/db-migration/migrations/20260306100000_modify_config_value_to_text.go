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
		Version: "20260306100000",
		Name:    "20260306100000_modify_config_value_to_text",
		Mode:    migrator.GormMode,
		Up:      mig20260306100000Up,
		Down:    mig20260306100000Down,
	})
}

func mig20260306100000Up(tx *gorm.DB) error {
	type Config struct {
		Value string `gorm:"column:value;type:text;NOT NULL"`
	}

	if tx.Migrator().HasColumn(&Config{}, "value") {
		if err := tx.Migrator().AlterColumn(&Config{}, "value"); err != nil {
			return err
		}
	}

	return nil
}

func mig20260306100000Down(tx *gorm.DB) error {
	type Config struct {
		Value string `gorm:"column:value;type:varchar(256);default:'';NOT NULL"`
	}

	if tx.Migrator().HasColumn(&Config{}, "value") {
		if err := tx.Migrator().AlterColumn(&Config{}, "value"); err != nil {
			return err
		}
	}

	return nil
}
