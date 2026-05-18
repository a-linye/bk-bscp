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
		Version: "20260518150700",
		Name:    "20260518150700_add_itsm_callback_token",
		Mode:    migrator.GormMode,
		Up:      mig20260518150700Up,
		Down:    mig20260518150700Down,
	})
}

func mig20260518150700Up(tx *gorm.DB) error {
	type strategie struct {
		ItsmCallbackToken string `gorm:"column:itsm_callback_token;type:varchar(256);default:NULL"`
	}

	if !tx.Migrator().HasColumn(&strategie{}, "itsm_callback_token") {
		if err := tx.Migrator().AddColumn(&strategie{}, "itsm_callback_token"); err != nil {
			return err
		}
	}

	return nil
}

func mig20260518150700Down(tx *gorm.DB) error {
	type strategie struct {
		ItsmCallbackToken string `gorm:"column:itsm_callback_token;type:varchar(256);default:NULL"`
	}

	if tx.Migrator().HasColumn(&strategie{}, "itsm_callback_token") {
		if err := tx.Migrator().DropColumn(&strategie{}, "itsm_callback_token"); err != nil {
			return err
		}
	}

	return nil
}
