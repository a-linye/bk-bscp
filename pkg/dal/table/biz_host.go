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

package table

import (
	"time"
)

// BizHost defines business host relationship
type BizHost struct {
	BizID       int       `db:"biz_id" json:"biz_id" gorm:"column:biz_id;primaryKey"`
	HostID      int       `db:"host_id" json:"host_id" gorm:"column:host_id;primaryKey"`
	AgentID     string    `db:"agent_id" json:"agent_id" gorm:"column:agent_id"`
	LastUpdated time.Time `db:"last_updated" json:"last_updated" gorm:"column:last_updated;autoUpdateTime"`
}

// TableName is the biz_host table name.
func (b *BizHost) TableName() Name {
	return BizHostTable
}
