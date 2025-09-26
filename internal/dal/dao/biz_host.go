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

package dao

import (
	"errors"

	"github.com/TencentBlueKing/bk-bscp/internal/dal/gen"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// BizHost supplies all the biz host related operations.
type BizHost interface {
	// Upsert upsert biz host relationship
	Upsert(kit *kit.Kit, bizHost *table.BizHost) error
	// BatchUpsert batch upsert biz host relationships
	BatchUpsert(kit *kit.Kit, bizHosts []*table.BizHost) error
	// List list biz host relationships
	List(kit *kit.Kit, bizID int) ([]*table.BizHost, error)
	// Delete delete biz host relationship
	Delete(kit *kit.Kit, bizID, hostID int) error
}

var _ BizHost = new(bizHostDao)

type bizHostDao struct {
	genQ *gen.Query
}

// Upsert upsert biz host relationship
func (dao *bizHostDao) Upsert(kit *kit.Kit, bizHost *table.BizHost) error {
	if bizHost == nil {
		return errors.New("biz host is nil")
	}

	m := dao.genQ.BizHost
	_, err := dao.genQ.BizHost.WithContext(kit.Ctx).
		Where(m.BizID.Eq(bizHost.BizID), m.HostID.Eq(bizHost.HostID)).
		Assign(m.AgentID.Value(bizHost.AgentID)).
		FirstOrCreate()

	return err
}

// BatchUpsert batch upsert biz host relationships
func (dao *bizHostDao) BatchUpsert(kit *kit.Kit, bizHosts []*table.BizHost) error {
	if len(bizHosts) == 0 {
		return nil
	}
	return dao.genQ.BizHost.WithContext(kit.Ctx).Save(bizHosts...)
}

// List list biz host relationships
func (dao *bizHostDao) List(kit *kit.Kit, bizID int) ([]*table.BizHost, error) {
	m := dao.genQ.BizHost
	return dao.genQ.BizHost.WithContext(kit.Ctx).
		Where(m.BizID.Eq(bizID)).
		Find()
}

// Delete delete biz host relationship
func (dao *bizHostDao) Delete(kit *kit.Kit, bizID, hostID int) error {
	m := dao.genQ.BizHost
	_, err := dao.genQ.BizHost.WithContext(kit.Ctx).
		Where(m.BizID.Eq(bizID), m.HostID.Eq(hostID)).
		Delete(&table.BizHost{})

	return err
}
