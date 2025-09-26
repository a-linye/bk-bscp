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

package crontab

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/components/bkcmdb"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/internal/runtime/shutdown"
	"github.com/TencentBlueKing/bk-bscp/internal/serviced"
	"github.com/TencentBlueKing/bk-bscp/internal/thirdparty/esb/cmdb"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
)

const (
	defaultSyncBizHostInterval = 7 * 24 * time.Hour // 一周一次全量数据同步
)

// NewSyncBizHost init sync biz host
func NewSyncBizHost(set dao.Set, sd serviced.Service, cmdbService bkcmdb.Service) SyncBizHost {
	return SyncBizHost{
		set:         set,
		state:       sd,
		cmdbService: cmdbService,
	}
}

// SyncBizHost sync business host relationship
type SyncBizHost struct {
	set         dao.Set
	state       serviced.Service
	cmdbService bkcmdb.Service
	mutex       sync.Mutex
}

// Run the sync biz host task
func (c *SyncBizHost) Run() {
	logs.Infof("start sync biz host task")
	notifier := shutdown.AddNotifier()
	go func() {
		ticker := time.NewTicker(defaultSyncBizHostInterval)
		defer ticker.Stop()
		for {
			kt := kit.New()
			ctx, cancel := context.WithCancel(kt.Ctx)
			kt.Ctx = ctx

			select {
			case <-notifier.Signal:
				logs.Infof("stop sync biz host success")
				cancel()
				notifier.Done()
				return
			case <-ticker.C:
				if !c.state.IsMaster() {
					logs.Infof("current service instance is slave, skip sync biz host")
					continue
				}
				logs.Infof("starts to synchronize the biz host")
				c.syncBizHost(kt)
			}
		}
	}()
}

// syncBizHost sync business host relationship
func (c *SyncBizHost) syncBizHost(kt *kit.Kit) {
	c.mutex.Lock()
	defer func() {
		c.mutex.Unlock()
	}()

	// 查询BSCP的业务
	// todo：当前接口返回的是全量业务，后续需要调整为仅查询使用BSCP的业务
	bizList, err := c.queryBSCPBusiness(kt)
	if err != nil {
		logs.Errorf("query BSCP business failed, err: %v", err)
		return
	}

	logs.Infof("found %d BSCP businesses", len(bizList))

	// 根据业务ID查询主机信息
	for _, biz := range bizList {
		if err := c.syncBusinessHosts(kt, biz); err != nil {
			logs.Errorf("sync business %d hosts failed, err: %v", biz.BizID, err)
			// todo： 确定同步失败后的处理方式
			continue
		}
	}

	logs.Infof("sync biz host completed")
}

// queryBSCPBusiness 查询BSCP的业务
func (c *SyncBizHost) queryBSCPBusiness(kt *kit.Kit) ([]*cmdb.Biz, error) {
	// 使用CMDB服务查询所有业务
	bizResult, err := c.cmdbService.ListAllBusiness(kt.Ctx)
	if err != nil {
		return nil, fmt.Errorf("list all business failed: %w", err)
	}

	// 转换为指针切片
	var bizList []*cmdb.Biz
	for i := range bizResult.Info {
		bizList = append(bizList, &bizResult.Info[i])
	}

	return bizList, nil
}

// syncBusinessHosts 同步单个业务的主机信息
func (c *SyncBizHost) syncBusinessHosts(kt *kit.Kit, biz *cmdb.Biz) error {
	// 查询业务下的主机
	// todo: 部分主机的agentid为空，可能需要过滤
	// todo：查询数量支持配置
	start := 0
	limit := 1000
	totalSynced := 0
	for {
		req := &bkcmdb.ListBizHostsRequest{
			BkBizID: int(biz.BizID),
			Page: bkcmdb.PageParam{
				Start: start,
				Limit: limit,
			},
			Fields: []string{"bk_biz_id", "bk_host_id", "bk_agent_id"},
		}

		hostResult, err := c.cmdbService.ListBizHosts(kt.Ctx, req)
		if err != nil {
			return fmt.Errorf("list biz hosts failed: %w", err)
		}

		if !hostResult.Result {
			return fmt.Errorf("list biz hosts failed: %s", hostResult.Message)
		}

		// 如果当前页没有数据，说明已经查询完毕
		if len(hostResult.Data.Info) == 0 {
			break
		}

		var batchBizHosts []*table.BizHost
		for _, host := range hostResult.Data.Info {
			bizHost := &table.BizHost{
				BizID:   int(biz.BizID),
				HostID:  host.BkHostID,
				AgentID: host.BkAgentID,
			}
			batchBizHosts = append(batchBizHosts, bizHost)
		}
		if len(batchBizHosts) > 0 {
			if err := c.set.BizHost().BatchUpsert(kt, batchBizHosts); err != nil {
				return fmt.Errorf("batch upsert biz hosts failed: %w", err)
			}
			totalSynced += len(batchBizHosts)
			logs.Infof("synced batch %d hosts for business %d (total: %d)", len(batchBizHosts), biz.BizID, totalSynced)
		}

		// 如果返回的数据少于limit，说明已经是最后一页
		if len(hostResult.Data.Info) < limit {
			break
		}

		// 准备查询下一页
		start += limit
	}

	logs.Infof("completed sync for business %d, total hosts: %d", biz.BizID, totalSynced)
	return nil
}
