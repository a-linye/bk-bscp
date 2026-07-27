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

// Package lcache provides a cache library for storing and retrieving data.
package lcache

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/bluele/gcache"
	prm "github.com/prometheus/client_golang/prometheus"

	clientset "github.com/TencentBlueKing/bk-bscp/cmd/feed-server/bll/client-set"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/constant"
	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/errf"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
	pbcs "github.com/TencentBlueKing/bk-bscp/pkg/protocol/cache-service"
	"github.com/TencentBlueKing/bk-bscp/pkg/runtime/jsoni"
	"github.com/TencentBlueKing/bk-bscp/pkg/tools"
	"github.com/TencentBlueKing/bk-bscp/pkg/types"
)

// newApp create an app meta's cache instance.
func newApp(mc *metric, cs *clientset.ClientSet) *App {
	app := new(App)
	app.mc = mc
	opt := cc.FeedServer().FSLocalCache
	metaClient := gcache.New(int(opt.AppCacheSize)).
		LRU().
		EvictedFunc(app.evictRecorder).
		Expiration(time.Duration(opt.AppCacheTTLSec) * time.Second).
		Build()

	idClient := gcache.New(int(opt.AppCacheSize)).
		LRU().
		EvictedFunc(app.evictRecorder).
		Expiration(time.Duration(opt.AppCacheTTLSec) * time.Second).
		Build()

	app.metaClient = metaClient
	app.idClient = idClient
	app.cs = cs
	app.defaultProjEnvCache = gcache.New(1024).
		LRU().
		Expiration(time.Duration(opt.AppCacheTTLSec) * time.Second).
		Build()

	return app
}

// App is the instance of the app cache.
type App struct {
	mc         *metric
	metaClient gcache.Cache
	idClient   gcache.Cache
	cs         *clientset.ClientSet

	// defaultProjEnvCache 缓存 bizID → (projectID, envID)，避免每次都 RPC 查询。
	// key: fmt.Sprintf("%d", bizID), value: [2]uint32{projectID, envID}
	defaultProjEnvCache gcache.Cache
}

// IsAppExist validate app if exist.
func (ap *App) IsAppExist(kt *kit.Kit, bizID, projectID, envID uint32, appIDs ...uint32) (bool, error) {
	if len(appIDs) == 0 {
		return false, errors.New("appID is required")
	}

	for index := range appIDs {
		_, err := ap.GetMeta(kt, bizID, projectID, envID, appIDs[index])
		if err != nil {
			if errf.Error(err).Code == errf.RecordNotFound {
				return false, nil
			}

			return false, err
		}
	}

	return true, nil
}

// RemoveCache 清空app缓存
func (ap *App) RemoveCache(kt *kit.Kit, bizID, projectID, envID uint32, appName string) {
	key := fmt.Sprintf("%d-%d-%d-%s", bizID, projectID, envID, appName)
	ap.idClient.Remove(key)

	// 强制 cacheserver 刷新缓存
	opt := &pbcs.GetAppIDReq{
		BizId:     bizID,
		AppName:   appName,
		Refresh:   true,
		ProjectId: projectID,
		EnvId:     envID,
	}

	_, _ = ap.cs.CS().GetAppID(kt.RpcCtx(), opt)
}

// ListApps 获取App列表, 不缓存，直接透传请求。
func (ap *App) ListApps(kt *kit.Kit, req *pbcs.ListAppsReq) (*pbcs.ListAppsResp, error) {
	// 使用 RpcCtx()，将 TenantID 等写入 outgoing metadata 传递给 cache-service
	return ap.cs.CS().ListApps(kt.RpcCtx(), req)
}

// GetAppID get app id by app name.
func (ap *App) GetAppID(kt *kit.Kit, bizID, projectID, envID uint32, appName string) (uint32, error) {
	key := fmt.Sprintf("%d-%d-%d-%s", bizID, projectID, envID, appName)
	val, err := ap.idClient.GetIFPresent(key)
	if err == nil {
		ap.mc.hitCounter.With(prm.Labels{"resource": "app_id", "biz": tools.Itoa(bizID)}).Inc()

		// hit from cache.
		appID, yes := val.(uint32)
		if !yes {
			return 0, fmt.Errorf("unsupported app id cache value type: %v", reflect.TypeOf(val).String())
		}
		return appID, nil
	}

	if err != gcache.KeyNotFoundError {
		// this is not a not found error, log it.
		logs.Errorf("get biz: %d, appName: %s app id from local cache failed, err: %v, rid: %s", bizID, appName,
			err, kt.Rid)
		// do not return here, try to refresh cache for now.
	}

	start := time.Now()
	// get the cache from cache service directly.
	opt := &pbcs.GetAppIDReq{
		BizId:     bizID,
		AppName:   appName,
		ProjectId: projectID,
		EnvId:     envID,
	}
	resp, err := ap.cs.CS().GetAppID(kt.RpcCtx(), opt)
	if err != nil {
		ap.mc.errCounter.With(prm.Labels{"resource": "app_id", "biz": tools.Itoa(bizID)}).Inc()
		return 0, err
	}

	err = ap.idClient.Set(key, resp.AppId)
	if err != nil {
		logs.Errorf("update biz: %d, appName: %s app id cache failed, err: %v, rid: %s", bizID, appName, err, kt.Rid)
		// do not return, ignore the error directly.
	}

	ap.mc.refreshLagMS.With(prm.Labels{"resource": "app_id", "biz": tools.Itoa(bizID)}).Observe(tools.SinceMS(start))

	return resp.AppId, nil
}

// GetMeta the app meta cache.
func (ap *App) GetMeta(kt *kit.Kit, bizID, projectID, envID uint32, appID uint32) (*types.AppCacheMeta, error) {
	val, err := ap.metaClient.GetIFPresent(appID)
	if err == nil {
		ap.mc.hitCounter.With(prm.Labels{"resource": "app_meta", "biz": tools.Itoa(bizID)}).Inc()

		// hit from cache.
		meta, yes := val.(*types.AppCacheMeta)
		if !yes {
			return nil, fmt.Errorf("unsupported app meta cache value type: %v", reflect.TypeOf(val).String())
		}
		return meta, nil
	}

	if err != gcache.KeyNotFoundError {
		// this is not a not found error, log it.
		logs.Errorf("get biz: %d, app: %d app meta from local cache failed, err: %v, rid: %s", bizID, appID,
			err, kt.Rid)
		// do not return here, try to refresh cache for now.
	}

	start := time.Now()
	// get the cache from cache service directly.
	opt := &pbcs.GetAppMetaReq{
		BizId:     bizID,
		AppId:     appID,
		ProjectId: projectID,
		EnvId:     envID,
	}

	resp, err := ap.cs.CS().GetAppMeta(kt.RpcCtx(), opt)
	if err != nil {
		ap.mc.errCounter.With(prm.Labels{"resource": "app_meta", "biz": tools.Itoa(bizID)}).Inc()
		return nil, err
	}

	meta := new(types.AppCacheMeta)
	err = jsoni.UnmarshalFromString(resp.JsonRaw, meta)
	if err != nil {
		return nil, err
	}

	err = ap.metaClient.Set(appID, meta)
	if err != nil {
		logs.Errorf("update biz: %d, app: %d cache failed, err: %v, rid: %s", bizID, appID, err, kt.Rid)
		// do not return, ignore the error directly.
	}

	ap.mc.refreshLagMS.With(prm.Labels{"resource": "app_meta", "biz": tools.Itoa(bizID)}).Observe(tools.SinceMS(start))

	return meta, nil
}

func (ap *App) delete(appID uint32) {
	ap.metaClient.Remove(appID)
}

// deleteByName 删除 idClient 中按名称索引的缓存，key 格式必须与 GetAppID/RemoveCache 的写入格式
// 保持一致（bizID-projectID-envID-appName），否则事件驱动的缓存失效会静默 miss。
// 事件的 attachment 可能未携带 project/env（为 0），此时先归一化为默认项目/环境再删。
func (ap *App) deleteByName(kt *kit.Kit, bizID, projectID, envID uint32, appName string) {
	if projectID == 0 || envID == 0 {
		projectID, envID = ap.ResolveProjectEnv(kt, bizID, projectID, envID)
	}
	key := fmt.Sprintf("%d-%d-%d-%s", bizID, projectID, envID, appName)
	ap.idClient.Remove(key)
}

func (ap *App) evictRecorder(key interface{}, _ interface{}) {
	appID, yes := key.(uint32)
	if !yes {
		return
	}

	ap.mc.evictCounter.With(prm.Labels{"resource": "app_meta"}).Inc()

	if logs.V(2) {
		logs.Infof("evict app meta cache, app: %d", appID)
	}
}

// nolint: unused
func (ap *App) collectHitRate() {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			ap.mc.hitRate.With(prm.Labels{"resource": "app_meta"}).Set(ap.metaClient.HitRate())
		}
	}()
}

// SetAppLastConsumedTime 设置服务拉取时间
func (ap *App) SetAppLastConsumedTime(kt *kit.Kit, bizID uint32, appIDs []uint32) error {
	if _, err := ap.cs.CS().SetAppLastConsumedTime(kt.RpcCtx(), &pbcs.SetAppLastConsumedTimeReq{
		BizId:  bizID,
		AppIds: appIDs,
	}); err != nil {
		return err
	}
	return nil
}

// HasBiz 业务是否存在
func (ap *App) HasBiz(kt *kit.Kit, bizID uint32) bool {
	key := fmt.Sprintf("%d-%s", bizID, "tenant-id")
	val, err := ap.idClient.GetIFPresent(key)
	if err == nil {
		ap.mc.hitCounter.With(prm.Labels{"resource": "tenant_id", "biz": tools.Itoa(bizID)}).Inc()

		// hit from cache.
		tenantID, yes := val.(string)
		if !yes {
			logs.Infof("unsupported app id cache value type: %v", reflect.TypeOf(val).String())
			return false
		}

		if len(tenantID) == 0 {
			return false
		}

		kt.TenantID = tenantID
		return true
	}

	if err != gcache.KeyNotFoundError {
		// this is not a not found error, log it.
		logs.Errorf("get biz: %d, tenant id from local cache failed, err: %v, rid: %s", bizID,
			err, kt.Rid)
		// do not return here, try to refresh cache for now.
	}

	start := time.Now()

	resp, err := ap.cs.CS().GetTenantIDByBiz(kt.RpcCtx(), &pbcs.GetTenantIDByBizReq{
		BizId:   bizID,
		Refresh: false,
	})

	if err != nil || len(resp.TenantId) == 0 {
		ap.mc.errCounter.With(prm.Labels{"resource": "tenant_id", "biz": tools.Itoa(bizID)}).Inc()
		logs.Errorf("get biz: %d, tenant id failed, err: %v, rid: %s", bizID,
			err, kt.Rid)
		return false
	}

	kt.TenantID = resp.TenantId

	err = ap.idClient.Set(key, resp.TenantId)
	if err != nil {
		logs.Errorf("update biz: %d, tenant id cache failed, err: %v, rid: %s", bizID, err, kt.Rid)
		// do not return, ignore the error directly.
	}

	ap.mc.refreshLagMS.With(prm.Labels{"resource": "tenant_id", "biz": tools.Itoa(bizID)}).Observe(tools.SinceMS(start))

	return len(resp.TenantId) != 0
}

// EnsureTenantID resolves the tenant ID for the given biz and sets it on the Kit.
// Uses local gcache first, falls back to cache-service RPC on miss.
func (ap *App) EnsureTenantID(kt *kit.Kit, bizID uint32) error {
	if kt.TenantID != "" {
		return nil
	}
	key := fmt.Sprintf("%d-%s", bizID, "tenant-id")
	val, err := ap.idClient.GetIFPresent(key)
	if err == nil {
		if tenantID, ok := val.(string); ok && tenantID != "" {
			kt.TenantID = tenantID
			return nil
		}
	}
	resp, err := ap.cs.CS().GetTenantIDByBiz(kt.RpcCtx(), &pbcs.GetTenantIDByBizReq{BizId: bizID})
	if err != nil {
		return err
	}
	tenantID := resp.TenantId
	if tenantID == "" {
		tenantID = constant.DefaultTenantID
	}
	kt.TenantID = tenantID
	_ = ap.idClient.Set(key, resp.TenantId)
	return nil
}

// ResolveProjectEnv 解析 projectID 和 environmentID，当任一为 0 时向 cache-service 查询默认值。
// feed-server 统一通过此方法确保传入 cache-service 的 project/env 总是非零（除非查询失败降级）。
// 调用方应优先从客户端请求中提取真实的 projectID/envID，仅在确实无法获取时才依赖此方法的默认值解析。
func (ap *App) ResolveProjectEnv(kt *kit.Kit, bizID, projectID, envID uint32) (uint32, uint32) {
	if projectID != 0 && envID != 0 {
		return projectID, envID
	}

	cacheKey := fmt.Sprintf("%d", bizID)
	val, err := ap.defaultProjEnvCache.GetIFPresent(cacheKey)
	if err == nil {
		if cached, ok := val.([2]uint32); ok {
			if cached[0] != 0 && cached[1] != 0 {
				logs.V(3).Infof("resolve default proj/env from local cache, biz: %d, proj: %d, env: %d",
					bizID, cached[0], cached[1])
				if projectID == 0 {
					projectID = cached[0]
				}
				if envID == 0 {
					envID = cached[1]
				}
				return projectID, envID
			}
		}
	}

	// 本地缓存未命中，调用 cache-service RPC 查询默认值
	resp, rpcErr := ap.cs.CS().GetDefaultProjectEnv(kt.RpcCtx(), &pbcs.GetDefaultProjectEnvReq{
		BizId: bizID,
	})
	if rpcErr != nil {
		logs.Errorf("get default project/env failed, biz: %d, err: %v, rid: %s, fallback to 0",
			bizID, rpcErr, kt.Rid)
		// 查询失败时保持原值（可能为 0），由 cache-service 端兜底处理
		return projectID, envID
	}

	// 仅填充调用方未显式指定的维度，显式传入的 projectID/envID 不能被默认值覆盖。
	if projectID == 0 && resp.ProjectId != 0 {
		projectID = resp.ProjectId
	}
	if envID == 0 && resp.EnvId != 0 {
		envID = resp.EnvId
	}

	// 缓存的必须是 RPC 返回的 biz 级默认值，而不是归一化后的调用方入参，
	// 否则会把某个请求显式指定的 project/env 错误固化成默认值污染后续请求。
	if resp.ProjectId != 0 && resp.EnvId != 0 {
		_ = ap.defaultProjEnvCache.Set(cacheKey, [2]uint32{resp.ProjectId, resp.EnvId})
	}

	logs.Infof("resolved default proj/env via RPC, biz: %d, proj: %d, env: %d", bizID, projectID, envID)
	return projectID, envID
}
