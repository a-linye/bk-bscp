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
	"time"

	"golang.org/x/time/rate"

	"github.com/TencentBlueKing/bk-bscp/cmd/data-service/service"
	"github.com/TencentBlueKing/bk-bscp/internal/components/bkcmdb"
	"github.com/TencentBlueKing/bk-bscp/internal/components/bkuser"
	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/internal/processor/processcheck"
	"github.com/TencentBlueKing/bk-bscp/internal/runtime/shutdown"
	"github.com/TencentBlueKing/bk-bscp/internal/serviced"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
)

const defaultCheckManagedInterval = 20 * time.Minute

// checkManagedProcess 进程托管配置定时检查任务
type checkManagedProcess struct {
	set      dao.Set
	state    serviced.Service
	svc      *service.Service
	cmdb     bkcmdb.Service
	runner   processcheck.ScriptRunner
	limiter  *rate.Limiter
	interval time.Duration
}

// NewCheckManagedProcess 构造定时检查任务
func NewCheckManagedProcess(set dao.Set, sd serviced.Service, svc *service.Service, cmdbSvc bkcmdb.Service,
	gseSvc *gse.Service, cfg cc.CheckProcessManagedConfig) *checkManagedProcess {
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		logs.Errorf("parse checkProcessManaged interval failed, using default: %v", err)
		interval = defaultCheckManagedInterval
	}

	return &checkManagedProcess{
		set:      set,
		state:    sd,
		svc:      svc,
		cmdb:     cmdbSvc,
		runner:   processcheck.NewGSEScriptRunner(gseSvc, cfg.LinuxProcScript, cfg.WindowsProcScript),
		limiter:  rate.NewLimiter(rate.Limit(cfg.QpsLimit), 1),
		interval: interval,
	}
}

// Run 启动周期任务：ticker + shutdown 通知 + IsMaster 守卫（slave 不下发任何脚本）。
func (c *checkManagedProcess) Run() {
	logs.Infof("[checkManagedProcess] start check managed process task, interval=%s", c.interval)
	notifier := shutdown.AddNotifier()
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			kt := kit.New()
			ctx, cancel := context.WithCancel(kt.Ctx)
			kt.Ctx = ctx

			select {
			case <-notifier.Signal:
				logs.Infof("[checkManagedProcess] stop check managed process task")
				cancel()
				notifier.Done()
				return
			case <-ticker.C:
				if !c.state.IsMaster() {
					logs.Infof("[checkManagedProcess] current instance is slave, skip check at=%s",
						time.Now().Format(time.RFC3339))
					continue
				}
				c.checkAllBiz(kt)
			}
		}
	}()
}

func (c *checkManagedProcess) checkAllBiz(kt *kit.Kit) {
	if cc.DataService().FeatureFlags.EnableMultiTenantMode {
		tenants, err := bkuser.ListEnabledTenants(kt.Ctx)
		if err != nil {
			logs.Errorf("[checkManagedProcess] list enabled tenants failed: %v", err)
			return
		}
		if len(tenants) == 0 {
			logs.Warnf("[checkManagedProcess] no enabled tenants found")
			return
		}
		for _, tenant := range tenants {
			c.checkBizByTenant(kit.NewWithTenant(tenant.ID))
		}
		return
	}

	c.checkBizByTenant(kt)
}

func (c *checkManagedProcess) checkBizByTenant(kt *kit.Kit) {
	start := time.Now()
	business, err := c.cmdb.SearchBusinessByAccount(kt.Ctx, bkcmdb.SearchSetReq{
		Fields: []string{"bk_biz_id"},
	})
	if err != nil {
		logs.Errorf("[checkManagedProcess] search business failed, tenant=%q: %v", kt.TenantID, err)
		return
	}

	checker := processcheck.NewChecker(c.set, c.runner, c.limiter)
	checked := 0
	for _, item := range business.Info {
		bizID := uint32(item.BkBizID)
		// 仅巡检开启进程配置管理的业务，跳过未开启项。
		if !c.svc.IsProcessConfigViewEnabled(bizID) {
			continue
		}
		if err := checker.CheckBiz(kt, bizID); err != nil {
			logs.Errorf("[checkManagedProcess] check biz %d (tenant=%q) failed: %v", bizID, kt.TenantID, err)
			continue
		}
		checked++
	}
	logs.Infof("[checkManagedProcess] check tenant=%q done, total=%d, checked=%d, cost=%s",
		kt.TenantID, len(business.Info), checked, time.Since(start))
}
