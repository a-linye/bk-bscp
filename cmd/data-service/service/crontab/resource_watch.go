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
	defaultWatchBizHostInterval = 30 * time.Second // Check events every 30 seconds
)

// NewWatchBizHost init watch biz host
func NewWatchBizHost(set dao.Set, sd serviced.Service, cmdbService bkcmdb.Service) WatchBizHost {
	timeAgo := time.Now().Add(-30 * time.Minute).Unix()
	return WatchBizHost{
		set:         set,
		state:       sd,
		cmdbService: cmdbService,
		startTime:   timeAgo,
		cursor:      "", // Initial cursor is empty
	}
}

// WatchBizHost watch business host relationship changes
type WatchBizHost struct {
	set         dao.Set
	state       serviced.Service
	cmdbService bkcmdb.Service
	mutex       sync.Mutex
	startTime   int64  // Start time for listening events
	cursor      string // Event cursor
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

// Event types
const (
	createEvent = "create"
	updateEvent = "update"
	// Delete events are not handled for now, handled through data scanning
	// deleteEvent = "delete"
)

// Resource types
const (
	hostRelation = "host_relation"
)

// watchBizHost watch business host relationship changes
func (w *WatchBizHost) watchBizHost(kt *kit.Kit) {
	w.mutex.Lock()
	defer func() {
		w.mutex.Unlock()
	}()

	// Listen to host relationship change events
	req := &bkcmdb.WatchResourceRequest{
		BkResource:   hostRelation, // Listen to host relationships
		BkEventTypes: []string{createEvent, updateEvent},
		BkFields:     []string{"bk_biz_id", "bk_host_id"},
	}
	if w.cursor != "" {
		// For non-first listening, use the previous cursor
		req.BkCursor = w.cursor
	} else {
		// For first listening, get events from the last 30 minutes
		req.BkStartFrom = &w.startTime
	}

	watchResult, err := w.cmdbService.WatchResource(kt.Ctx, req)
	if err != nil {
		logs.Errorf("watch resource failed, err: %v", err)
		return
	}

	if !watchResult.Result {
		logs.Errorf("watch resource failed: %s", watchResult.Message)
		return
	}
	if !watchResult.Data.BkWatched {
		// No events found, skip
		logs.Infof("no events found")
		return
	}

	// Process events
	if len(watchResult.Data.BkEvents) > 0 {
		if err := w.processEvents(kt, watchResult.Data.BkEvents); err != nil {
			logs.Errorf("process events failed, err: %v", err)
			return
		}
		// Update cursor to the last event's cursor
		lastEvent := watchResult.Data.BkEvents[len(watchResult.Data.BkEvents)-1]
		w.cursor = lastEvent.BkCursor
	}
}

// processEvents process event list
func (w *WatchBizHost) processEvents(kt *kit.Kit, events []bkcmdb.HostRelationEvent) error {
	for _, event := range events {
		if err := w.processEvent(kt, event); err != nil {
			logs.Errorf("process event failed, event: %+v, err: %v", event, err)
			// Skip failed events, rely on full data sync and other fallback measures
			continue
		}
	}
	return nil
}

// processEvent process single event
func (w *WatchBizHost) processEvent(kt *kit.Kit, event bkcmdb.HostRelationEvent) error {
	switch event.BkEventType {
	case createEvent, updateEvent:
		return w.handleHostRelationEvent(kt, event)
	default:
		logs.Warnf("unknown event type: %s", event.BkEventType)
		return nil
	}
}

// handleHostRelationEvent handle host relation event
func (w *WatchBizHost) handleHostRelationEvent(kt *kit.Kit, event bkcmdb.HostRelationEvent) error {
	if event.BkDetail == nil {
		logs.Warnf("host relation event has nil detail, skipping")
		return nil
	}

	detail := event.BkDetail
	if detail.BkBizID == nil || detail.BkHostID == nil {
		logs.Warnf("invalid host relation event detail: %+v", detail)
		return nil
	}

	bizHost := &table.BizHost{
		BizID:  *detail.BkBizID,
		HostID: *detail.BkHostID,
	}

	if err := w.set.BizHost().Upsert(kt, bizHost); err != nil {
		return fmt.Errorf("upsert biz[%d] host[%d] failed: %w", detail.BkBizID, detail.BkHostID, err)
	}
	return nil
}
