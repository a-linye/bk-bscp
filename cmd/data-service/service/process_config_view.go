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

package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
	pbbase "github.com/TencentBlueKing/bk-bscp/pkg/protocol/core/base"
	pbds "github.com/TencentBlueKing/bk-bscp/pkg/protocol/data-service"
)

const (
	pcvKeyPrefix       = "pcv_biz:"
	pcvRefreshInterval = 5 * time.Minute
)

// PcvCache 进程与配置管理可见性缓存
type PcvCache struct {
	mu   sync.RWMutex
	spec map[uint32]bool // bizID -> enabled
}

// Get returns the enabled state for a biz, and whether the entry exists.
func (c *PcvCache) Get(bizID uint32) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.spec[bizID]
	return v, ok
}

// Set updates an entry in the cache.
func (c *PcvCache) Set(bizID uint32, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spec[bizID] = enabled
}

// Delete removes an entry from the cache.
func (c *PcvCache) Delete(bizID uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.spec, bizID)
}

// Replace replaces the entire cache content.
func (c *PcvCache) Replace(m map[uint32]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spec = m
}

func pcvKey(bizID uint32) string {
	return pcvKeyPrefix + strconv.FormatUint(uint64(bizID), 10)
}

func parsePcvKey(key string) (uint32, error) {
	s := strings.TrimPrefix(key, pcvKeyPrefix)
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid pcv key %q: %w", key, err)
	}
	return uint32(id), nil
}

// InitPcvCache initializes the process config view cache and starts background refresh.
func (s *Service) InitPcvCache() {
	s.pcvCache = &PcvCache{spec: make(map[uint32]bool)}
	if err := s.refreshPcvCache(); err != nil {
		logs.Errorf("initial pcv cache load failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.pcvCacheCancel = cancel
	go s.pcvCacheRefreshLoop(ctx)
}

// StopPcvCache stops the background pcv cache refresh goroutine.
func (s *Service) StopPcvCache() {
	if s.pcvCacheCancel != nil {
		s.pcvCacheCancel()
	}
}

func (s *Service) refreshPcvCache() error {
	kt := kit.New()
	configs, err := s.dao.Config().ListConfigByKeyPrefix(kt, pcvKeyPrefix)
	if err != nil {
		return fmt.Errorf("list pcv configs: %w", err)
	}
	m := make(map[uint32]bool, len(configs))
	for _, c := range configs {
		bizID, e := parsePcvKey(c.Key)
		if e != nil {
			logs.Warnf("skip invalid pcv config key %q: %v", c.Key, e)
			continue
		}
		m[bizID] = c.Value == "true"
	}
	s.pcvCache.Replace(m)
	return nil
}

func (s *Service) pcvCacheRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(pcvRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logs.Infof("pcv cache refresh loop stopped")
			return
		case <-ticker.C:
			if err := s.refreshPcvCache(); err != nil {
				logs.Errorf("pcv cache refresh failed: %v", err)
			}
		}
	}
}

// IsProcessConfigViewEnabled checks whether process config view is enabled for a given biz.
func (s *Service) IsProcessConfigViewEnabled(bizID uint32) bool {
	if s.pcvCache == nil {
		return false
	}
	enabled, ok := s.pcvCache.Get(bizID)
	if !ok {
		return false
	}
	return enabled
}

// UpsertBizProcessConfigView creates or updates a biz's process config view setting.
func (s *Service) UpsertBizProcessConfigView(ctx context.Context,
	req *pbds.UpsertBizProcessConfigViewReq) (*pbbase.EmptyResp, error) {

	kt := kit.FromGrpcContext(ctx)

	val := "false"
	if req.Enabled {
		val = "true"
	}

	if err := s.dao.Config().UpsertConfig(kt, []*table.Config{
		{Key: pcvKey(req.BizId), Value: val},
	}); err != nil {
		logs.Errorf("upsert pcv config for biz %d failed: %v, rid: %s", req.BizId, err, kt.Rid)
		return nil, err
	}

	s.pcvCache.Set(req.BizId, req.Enabled)

	return &pbbase.EmptyResp{}, nil
}

// DeleteBizProcessConfigView removes a biz's process config view setting.
func (s *Service) DeleteBizProcessConfigView(ctx context.Context,
	req *pbds.DeleteBizProcessConfigViewReq) (*pbbase.EmptyResp, error) {

	kt := kit.FromGrpcContext(ctx)

	if err := s.dao.Config().DeleteConfigByKey(kt, pcvKey(req.BizId)); err != nil {
		logs.Errorf("delete pcv config for biz %d failed: %v, rid: %s", req.BizId, err, kt.Rid)
		return nil, err
	}

	s.pcvCache.Delete(req.BizId)

	return &pbbase.EmptyResp{}, nil
}

// ListBizProcessConfigView lists configured biz process config view entries.
func (s *Service) ListBizProcessConfigView(ctx context.Context,
	req *pbds.ListBizProcessConfigViewReq) (*pbds.ListBizProcessConfigViewResp, error) {

	if req.BizId > 0 {
		enabled, ok := s.pcvCache.Get(req.BizId)
		if !ok {
			return &pbds.ListBizProcessConfigViewResp{}, nil
		}
		return &pbds.ListBizProcessConfigViewResp{
			Items: []*pbds.BizProcessConfigViewItem{{BizId: req.BizId, Enabled: enabled}},
		}, nil
	}

	kt := kit.FromGrpcContext(ctx)

	configs, err := s.dao.Config().ListConfigByKeyPrefix(kt, pcvKeyPrefix)
	if err != nil {
		logs.Errorf("list pcv configs failed: %v, rid: %s", err, kt.Rid)
		return nil, err
	}

	items := make([]*pbds.BizProcessConfigViewItem, 0, len(configs))
	for _, c := range configs {
		bizID, e := parsePcvKey(c.Key)
		if e != nil {
			continue
		}
		items = append(items, &pbds.BizProcessConfigViewItem{
			BizId:   bizID,
			Enabled: c.Value == "true",
		})
	}

	return &pbds.ListBizProcessConfigViewResp{Items: items}, nil
}
