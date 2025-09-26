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
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
)

const (
	defaultWatchBizHostInterval = 30 * time.Second // 30秒检查一次事件
)

// NewWatchBizHost init watch biz host
func NewWatchBizHost(set dao.Set, sd serviced.Service, cmdbService bkcmdb.Service) WatchBizHost {
	return WatchBizHost{
		set:         set,
		state:       sd,
		cmdbService: cmdbService,
		cursor:      "", // 初始cursor为空
	}
}

// WatchBizHost watch business host relationship changes
type WatchBizHost struct {
	set         dao.Set
	state       serviced.Service
	cmdbService bkcmdb.Service
	mutex       sync.Mutex
	cursor      string // 事件游标
}

// Run the watch biz host task
func (w *WatchBizHost) Run() {
	logs.Infof("start watch biz host task")
	notifier := shutdown.AddNotifier()
	go func() {
		ticker := time.NewTicker(defaultWatchBizHostInterval)
		defer ticker.Stop()
		for {
			kt := kit.New()
			ctx, cancel := context.WithCancel(kt.Ctx)
			kt.Ctx = ctx

			select {
			case <-notifier.Signal:
				logs.Infof("stop watch biz host success")
				cancel()
				notifier.Done()
				return
			case <-ticker.C:
				if !w.state.IsMaster() {
					logs.Infof("current service instance is slave, skip watch biz host")
					continue
				}
				logs.Infof("starts to watch biz host changes")
				w.watchBizHost(kt)
			}
		}
	}()
}

// watchBizHost watch business host relationship changes
func (w *WatchBizHost) watchBizHost(kt *kit.Kit) {
	w.mutex.Lock()
	defer func() {
		w.mutex.Unlock()
	}()

	// 监听主机关系变化事件
	req := &bkcmdb.WatchResourceRequest{
		BkResource:   "host_relation", // 监听主机关系
		BkEventTypes: []string{"create", "update", "delete"},
		BkFields:     []string{"bk_biz_id", "bk_host_id"},
		BkCursor:     w.cursor, // 使用上次的cursor
	}

	watchResult, err := w.cmdbService.WatchResource(kt.Ctx, req)
	if err != nil {
		logs.Errorf("watch resource failed, err: %v", err)
		// todo：处理失败情况
		return
	}

	if !watchResult.Result {
		logs.Errorf("watch resource failed: %s", watchResult.Message)
		// todo: 如果是业务错误，可能需要重置cursor或跳过
		return
	}

	// 处理事件
	if len(watchResult.Data.BkEvents) > 0 {
		if err := w.processEvents(kt, watchResult.Data.BkEvents); err != nil {
			logs.Errorf("process events failed, err: %v", err)
			return
		}
		logs.Infof("processed %d events", len(watchResult.Data.BkEvents))

		// 更新cursor为最后一个事件的cursor
		lastEvent := watchResult.Data.BkEvents[len(watchResult.Data.BkEvents)-1]
		w.cursor = lastEvent.BkCursor
		logs.Infof("updated cursor to: %s", w.cursor)
	}
}

// processEvents 处理事件列表
func (w *WatchBizHost) processEvents(kt *kit.Kit, events []bkcmdb.HostRelationEvent) error {
	for _, event := range events {
		if err := w.processEvent(kt, event); err != nil {
			logs.Errorf("process event failed, event: %+v, err: %v", event, err)
			// 事件处理失败则跳过，后续依赖全量数据同步和其他兜底措施处理
			continue
		}
	}
	return nil
}

// 事件类型
const (
	createEvent = "create"
	updateEvent = "update"
	deleteEvent = "delete"
)

// processEvent 处理单个事件
func (w *WatchBizHost) processEvent(kt *kit.Kit, event bkcmdb.HostRelationEvent) error {
	switch event.BkEventType {
	case createEvent:
		return w.handleHostRelationCreateEvent(kt, event)
	case updateEvent:
		return w.handleHostRelationUpdateEvent(kt, event)
	case deleteEvent:
		return w.handleHostRelationDeleteEvent(kt, event)
	default:
		logs.Warnf("unknown event type: %s", event.BkEventType)
		return nil
	}
}

// handleHostRelationCreateEvent 处理创建事件
func (w *WatchBizHost) handleHostRelationCreateEvent(kt *kit.Kit, event bkcmdb.HostRelationEvent) error {
	detail := event.BkDetail
	// 新增的主机关系事件不一定是主机和业务的关系事件，因此采取存在则更新的操作
	return w.upsertBizHostRelation(kt, detail.BkBizID, detail.BkHostID, event.BkEventType)
}

// handleHostRelationUpdateEvent 处理更新事件
func (w *WatchBizHost) handleHostRelationUpdateEvent(kt *kit.Kit, event bkcmdb.HostRelationEvent) error {
	detail := event.BkDetail
	return w.upsertBizHostRelation(kt, detail.BkBizID, detail.BkHostID, event.BkEventType)
}

// handleHostRelationDeleteEvent 处理删除事件
func (w *WatchBizHost) handleHostRelationDeleteEvent(kt *kit.Kit, event bkcmdb.HostRelationEvent) error {
	detail := event.BkDetail
	return w.deleteBizHostRelation(kt, detail.BkBizID, detail.BkHostID)
}

// upsertBizHost 插入或更新业务主机关系关系
func (w *WatchBizHost) upsertBizHostRelation(kt *kit.Kit, bizID, hostID int, eventType string) error {
	bizHost := &table.BizHost{
		BizID:  bizID,
		HostID: hostID,
	}

	if err := w.set.BizHost().Upsert(kt, bizHost); err != nil {
		return fmt.Errorf("upsert biz[%d] host[%d] failed: %w", bizID, hostID, err)
	}

	logs.Infof("%sed biz host relationship: biz_id=%d, host_id=%d", eventType, bizID, hostID)
	return nil
}

// deleteBizHost 删除业务主机关系关系
func (w *WatchBizHost) deleteBizHostRelation(kt *kit.Kit, bizID, hostID int) error {
	if err := w.set.BizHost().Delete(kt, bizID, hostID); err != nil {
		return fmt.Errorf("delete biz[%d] host[%d] failed: %w", bizID, hostID, err)
	}

	logs.Infof("deleted biz host relationship: biz_id=%d, host_id=%d", bizID, hostID)
	return nil
}
