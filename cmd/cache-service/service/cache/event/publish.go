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

// Package event handle publish
package event

import (
	"context"
	"fmt"
	"time"

	"github.com/TencentBlueKing/bk-bscp/cmd/cache-service/service/cache/client"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/bedis"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/internal/runtime/shutdown"
	"github.com/TencentBlueKing/bk-bscp/internal/serviced"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
	"github.com/TencentBlueKing/bk-bscp/pkg/types"
)

const (
	defaultPublishInterval = 1 * time.Second
)

// Publish xxx
type Publish struct {
	set   dao.Set
	state serviced.State
	bds   bedis.Client
	op    client.Interface
}

// NewPublish init publish
func NewPublish(set dao.Set, state serviced.State, bds bedis.Client, op client.Interface) Publish {
	return Publish{
		set:   set,
		state: state,
		bds:   bds,
		op:    op,
	}
}

// Run the publish task
func (cm *Publish) Run() {
	logs.Infof("start publish task")
	notifier := shutdown.AddNotifier()
	go func() {
		ticker := time.NewTicker(defaultPublishInterval)
		defer ticker.Stop()
		for {
			kt := kit.New()
			ctx, cancel := context.WithCancel(kt.Ctx)
			kt.Ctx = ctx

			select {
			case <-notifier.Signal:
				logs.Infof("stop handle client publish data success")
				cancel()
				notifier.Done()
				return
			case <-ticker.C:
				logs.Infof("start handle client publish data")

				if !cm.state.IsMaster() {
					logs.V(2).Infof("this is slave, do not need to handle, skip. rid: %s", kt.Rid)
					time.Sleep(sleepTime)
					continue
				}
				cm.updateStrategy(kt)
			}
		}
	}()
}

// 上线更新状态
// nolint funlen
func (cm *Publish) updateStrategy(kt *kit.Kit) {
	locateTime := time.Now().UTC()
	publishInfos, err := cm.op.GetPublishTime(kt, locateTime.Unix())
	if err != nil {
		logs.Errorf("get publish time failed, err: %s, rid: %s", err.Error(), kt.Rid)
		return
	}

	if len(publishInfos) == 0 {
		return
	}

	var strategyIds []uint32
	zrems := make(map[string][]string)
	for k, v := range publishInfos {
		strategyIds = append(strategyIds, k)
		zrems[v.Key] = append(zrems[v.Key], fmt.Sprintf("%d", k))
	}

	strategies, err := cm.set.Strategy().GetStrategyByIDs(kt.WithSkipTenantFilter(), strategyIds)
	if err != nil {
		logs.Errorf("get strategy by ids failed, err: %s, rid: %s", err.Error(), kt.Rid)
		return
	}

	// 按 TenantID 分组: manualStrategies 和 publishStrategies
	type tenantGroup struct {
		manualIDs  []uint32
		publishIDs []uint32
	}
	tenantGroups := make(map[string]*tenantGroup)

	tx := cm.set.GenQuery().Begin()
	defer func() {
		if rErr := tx.Rollback(); rErr != nil {
			logs.Errorf("transaction rollback failed, err: %v, rid: %s", rErr, kt.Rid)
		}
	}()

	for _, v := range strategies {
		// 类型必须是定时上线
		if v.Spec.PublishType == table.Scheduled {
			tenantID := v.Attachment.TenantID
			if _, ok := tenantGroups[tenantID]; !ok {
				tenantGroups[tenantID] = &tenantGroup{}
			}
			group := tenantGroups[tenantID]

			// 刚好到上线时间未审批的情况，更新为手动上线
			if v.Spec.PublishStatus == table.PendingApproval {
				group.manualIDs = append(group.manualIDs, v.ID)
				continue
			}

			// 因环境问题未上线的，要么刚好到上线时间未审批的情况，要么刚好要上线没上线
			if v.Spec.PublishStatus == table.PendingPublish {
				// 需要审批的更新为手动上线
				if v.Spec.FinalApprovalTime.Unix() > publishInfos[v.ID].PublishTime && v.Spec.Approver != "" {
					group.manualIDs = append(group.manualIDs, v.ID)
					continue
				}
				// 刚好要上线，以及因环境问题刚好要上线没上线
				group.publishIDs = append(group.publishIDs, v.ID)

				opt := types.PublishOption{
					BizID:     v.Attachment.BizID,
					AppID:     v.Attachment.AppID,
					ReleaseID: v.Spec.ReleaseID,
					All:       false,
				}

				if len(v.Spec.Scope.Groups) == 0 {
					opt.All = true
				}

				tkt := kit.NewWithTenant(tenantID)
				tkt.User = v.Revision.Creator
				err = cm.set.Publish().UpsertPublishWithTx(tkt, tx, &opt, v)
				if err != nil {
					logs.Errorf("update publish with tx failed, err: %s, rid: %s", err.Error(), tkt.Rid)
					return
				}
			}
		}
	}

	// 按租户分组执行 UpdateByIDs，确保每次操作的 kit 携带正确的 TenantID
	for tenantID, group := range tenantGroups {
		tkt := kit.NewWithTenant(tenantID)

		err = cm.set.Strategy().UpdateByIDs(tkt, tx, group.manualIDs, map[string]interface{}{
			"publish_type":        table.Manually,
			"final_approval_time": time.Now().UTC(),
		})
		if err != nil {
			logs.Errorf("update strategy by ids manually failed, err: %s, rid: %s", err.Error(), tkt.Rid)
			return
		}

		err = cm.set.Strategy().UpdateByIDs(tkt, tx, group.publishIDs, map[string]interface{}{
			"publish_status":      table.AlreadyPublish,
			"pub_state":           table.Publishing,
			"final_approval_time": time.Now().UTC(),
		})
		if err != nil {
			logs.Errorf("update strategy by ids already publish failed, err: %s, rid: %s", err.Error(), tkt.Rid)
			return
		}

		// update audit details
		err = cm.set.AuditDao().UpdateByStrategyIDs(tkt, tx, group.publishIDs, map[string]interface{}{
			"status": table.AlreadyPublish,
		})
		if err != nil {
			logs.Errorf("update audit by strategy ids failed, err: %s, rid: %s", err.Error(), tkt.Rid)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		logs.Errorf("commit transaction failed, err: %v, rid: %s", err, kt.Rid)
		return
	}

	for k, v := range zrems {
		_, err := cm.bds.ZRem(kt.Ctx, k, v)
		if err != nil {
			logs.Errorf("zrem failed, err: %v, rid: %s", err, kt.Rid)
			return
		}
	}
}
