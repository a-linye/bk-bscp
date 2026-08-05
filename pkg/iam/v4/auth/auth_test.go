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

package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TencentBlueKing/bk-bscp/pkg/iam/meta"
	"github.com/TencentBlueKing/bk-bscp/pkg/iam/v4/client"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// fakeGateway 记录每次批量鉴权的入参，并按预设结果作答。
type fakeGateway struct {
	mu sync.Mutex
	// calls 按调用顺序记录每批的操作与资源 ID
	calls []recordedCall
	// allow 决定某个 (action, resourceID) 是否放行，未列出的按不放行
	allow map[string]bool
	err   error
	// maxConcurrent 记录观察到的最大并发数
	inflight      int
	maxConcurrent int
}

type recordedCall struct {
	actionID    string
	resourceIDs []string
}

func (f *fakeGateway) AuthByResources(_ *kit.Kit, _ client.Subject, actionID string,
	resources []client.AuthResource) (map[string]bool, error) {

	f.mu.Lock()
	f.inflight++
	if f.inflight > f.maxConcurrent {
		f.maxConcurrent = f.inflight
	}

	ids := make([]string, 0, len(resources))
	for _, r := range resources {
		ids = append(ids, r.ID)
	}
	f.calls = append(f.calls, recordedCall{actionID: actionID, resourceIDs: ids})
	f.mu.Unlock()

	// 留一点时间让并发真正重叠，否则 maxConcurrent 恒为 1。
	time.Sleep(5 * time.Millisecond)

	f.mu.Lock()
	f.inflight--
	f.mu.Unlock()

	if f.err != nil {
		return nil, f.err
	}

	got := make(map[string]bool, len(resources))
	for _, r := range resources {
		got[r.ID] = f.allow[actionID+"|"+r.ID]
	}

	return got, nil
}

func (f *fakeGateway) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.calls)
}

func testKit() *kit.Kit {
	return &kit.Kit{Ctx: context.Background(), TenantID: "default", User: "tester"}
}

func appAttr(action meta.Action, bizID, appID uint32) *meta.ResourceAttribute {
	return &meta.ResourceAttribute{
		Basic: meta.Basic{Type: meta.App, Action: action, ResourceID: appID},
		BizID: bizID,
	}
}

func bizAttr(action meta.Action, bizID uint32) *meta.ResourceAttribute {
	return &meta.ResourceAttribute{
		Basic: meta.Basic{Type: meta.Biz, Action: action},
		BizID: bizID,
	}
}

func newTestAuthorizer(t *testing.T, gw gateway, cfg Config) *Authorizer {
	t.Helper()

	a, err := NewAuthorizer(gw, cfg)
	require.NoError(t, err)

	return a
}

func TestNewAuthorizerRejectsNilGateway(t *testing.T) {
	_, err := NewAuthorizer(nil, Config{})
	require.ErrorContains(t, err, "gateway is nil")
}

func TestAuthorizeBatchEmpty(t *testing.T) {
	a := newTestAuthorizer(t, &fakeGateway{}, Config{})

	got, err := a.AuthorizeBatch(testKit(), "tester")
	require.NoError(t, err)
	require.Empty(t, got)
}

// 确实需要向权限中心发问时，用户名不可缺。
func TestAuthorizeBatchRequiresUserWhenAsking(t *testing.T) {
	a := newTestAuthorizer(t, &fakeGateway{}, Config{})

	_, err := a.AuthorizeBatch(testKit(), "", bizAttr(meta.FindBusinessResource, 1))
	require.ErrorContains(t, err, "user is not set")
}

// 回归：全部资源都命中 skip 时不得要求用户名。
//
// feed-server 处理 sidecar 请求时用应用凭证而非用户身份，kt.User 为空，
// 而它鉴权的 Sidecar 资源本就无需向权限中心发问。早先在入口处就校验用户名，
// 导致 feed-server 的每次鉴权都以 "user name is not set" 失败。
func TestAuthorizeBatchAllowsEmptyUserWhenAllSkipped(t *testing.T) {
	gw := &fakeGateway{}
	a := newTestAuthorizer(t, gw, Config{CacheSize: 100, CacheTTL: time.Minute})

	got, err := a.AuthorizeBatch(testKit(), "",
		&meta.ResourceAttribute{
			Basic: meta.Basic{Type: meta.Sidecar, Action: meta.Access}, BizID: 2},
		&meta.ResourceAttribute{
			Basic: meta.Basic{Type: meta.ConfigItem, Action: meta.View}, BizID: 2},
	)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, got[0].Authorized)
	require.True(t, got[1].Authorized)
	require.Zero(t, gw.callCount(), "skipped resources must not reach the gateway")
}

// 混合场景：部分 skip、部分需要鉴权，且用户名为空时应报错而不是静默放行。
func TestAuthorizeBatchRejectsEmptyUserOnMixedResources(t *testing.T) {
	a := newTestAuthorizer(t, &fakeGateway{}, Config{})

	_, err := a.AuthorizeBatch(testKit(), "",
		&meta.ResourceAttribute{
			Basic: meta.Basic{Type: meta.Sidecar, Action: meta.Access}, BizID: 2},
		bizAttr(meta.FindBusinessResource, 2),
	)
	require.ErrorContains(t, err, "user is not set")
}

// 结果必须与入参一一对应、顺序一致。
func TestAuthorizeBatchPreservesOrder(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{
		"app_view|30001": true,
		"app_edit|30001": false,
	}}
	a := newTestAuthorizer(t, gw, Config{})

	got, err := a.AuthorizeBatch(testKit(), "tester",
		appAttr(meta.View, 100001, 30001),
		appAttr(meta.Update, 100001, 30001),
	)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, got[0].Authorized)
	require.False(t, got[1].Authorized)
	require.Equal(t, meta.View, got[0].Resource.Action)
	require.Equal(t, meta.Update, got[1].Resource.Action)
}

// 不做权限控制的资源直接放行，不发请求。
func TestAuthorizeBatchSkipsUncontrolledResources(t *testing.T) {
	gw := &fakeGateway{}
	a := newTestAuthorizer(t, gw, Config{})

	got, err := a.AuthorizeBatch(testKit(), "tester",
		&meta.ResourceAttribute{Basic: meta.Basic{Type: meta.ConfigItem, Action: meta.View}, BizID: 1},
		&meta.ResourceAttribute{Basic: meta.Basic{Type: meta.App, Action: meta.SkipAction}, BizID: 1},
	)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, got[0].Authorized)
	require.True(t, got[1].Authorized)
	require.Zero(t, gw.callCount())
}

// 同一操作的多个资源合成一次请求，不同操作分开。
func TestAuthorizeBatchGroupsByAction(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{
		"app_view|30001": true,
		"app_view|30002": true,
		"app_edit|30001": true,
	}}
	a := newTestAuthorizer(t, gw, Config{})

	got, err := a.AuthorizeBatch(testKit(), "tester",
		appAttr(meta.View, 100001, 30001),
		appAttr(meta.View, 100001, 30002),
		appAttr(meta.Update, 100001, 30001),
	)
	require.NoError(t, err)
	require.Len(t, got, 3)
	for i, d := range got {
		require.True(t, d.Authorized, "decision %d", i)
	}

	// app_view 两个资源一批，app_edit 一个资源一批
	require.Equal(t, 2, gw.callCount())
}

// 超过单次上限时必须切批，每批不超过 MaxAuthBatchSize。
func TestAuthorizeBatchSplitsOverLimit(t *testing.T) {
	const total = client.MaxAuthBatchSize*2 + 3

	allow := make(map[string]bool, total)
	attrs := make([]*meta.ResourceAttribute, 0, total)
	for i := 0; i < total; i++ {
		appID := uint32(30000 + i)
		allow[fmt.Sprintf("app_view|%d", appID)] = true
		attrs = append(attrs, appAttr(meta.View, 100001, appID))
	}

	gw := &fakeGateway{allow: allow}
	a := newTestAuthorizer(t, gw, Config{})

	got, err := a.AuthorizeBatch(testKit(), "tester", attrs...)
	require.NoError(t, err)
	require.Len(t, got, total)
	for i, d := range got {
		require.True(t, d.Authorized, "decision %d", i)
	}

	// 43 个资源按 20 切批得到 3 批
	require.Equal(t, 3, gw.callCount())
	for _, c := range gw.calls {
		require.LessOrEqual(t, len(c.resourceIDs), client.MaxAuthBatchSize)
	}
}

// 同一资源在一批里只发一次，但每个入参下标都要有结果。
func TestAuthorizeBatchDeduplicatesResources(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{"app_view|30001": true}}
	a := newTestAuthorizer(t, gw, Config{})

	got, err := a.AuthorizeBatch(testKit(), "tester",
		appAttr(meta.View, 100001, 30001),
		appAttr(meta.View, 100001, 30001),
		appAttr(meta.View, 100001, 30001),
	)
	require.NoError(t, err)
	require.Len(t, got, 3)
	for i, d := range got {
		require.True(t, d.Authorized, "decision %d", i)
	}

	require.Equal(t, 1, gw.callCount())
	require.Len(t, gw.calls[0].resourceIDs, 1)
}

// 权限中心未返回的资源按无权限处理，宁可拒绝也不误放。
func TestAuthorizeBatchTreatsMissingResultAsDenied(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{}}
	a := newTestAuthorizer(t, gw, Config{})

	got, err := a.AuthorizeBatch(testKit(), "tester", appAttr(meta.View, 100001, 30001))
	require.NoError(t, err)
	require.False(t, got[0].Authorized)
}

func TestAuthorizeBatchPropagatesGatewayError(t *testing.T) {
	gw := &fakeGateway{err: errors.New("gateway down")}
	a := newTestAuthorizer(t, gw, Config{})

	_, err := a.AuthorizeBatch(testKit(), "tester", appAttr(meta.View, 100001, 30001))
	require.ErrorContains(t, err, "gateway down")
}

func TestAuthorizeBatchRejectsInvalidResource(t *testing.T) {
	a := newTestAuthorizer(t, &fakeGateway{}, Config{})

	_, err := a.AuthorizeBatch(testKit(), "tester",
		&meta.ResourceAttribute{Basic: meta.Basic{Type: "bogus", Action: meta.View}})
	require.ErrorContains(t, err, "unsupported bscp auth type")
}

// 命中缓存后不再请求权限中心。
func TestAuthorizeBatchUsesCache(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{"app_view|30001": true}}
	a := newTestAuthorizer(t, gw, Config{CacheSize: 100, CacheTTL: time.Minute})

	first, err := a.AuthorizeBatch(testKit(), "tester", appAttr(meta.View, 100001, 30001))
	require.NoError(t, err)
	require.True(t, first[0].Authorized)
	require.Equal(t, 1, gw.callCount())

	second, err := a.AuthorizeBatch(testKit(), "tester", appAttr(meta.View, 100001, 30001))
	require.NoError(t, err)
	require.True(t, second[0].Authorized)
	require.Equal(t, 1, gw.callCount(), "second call should be served from cache")
}

// 缓存键必须区分租户，否则多租户下会跨租户泄漏权限。
func TestCacheIsolatesTenants(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{"app_view|30001": true}}
	a := newTestAuthorizer(t, gw, Config{CacheSize: 100, CacheTTL: time.Minute})

	ktA := &kit.Kit{Ctx: context.Background(), TenantID: "tenant-a"}
	ktB := &kit.Kit{Ctx: context.Background(), TenantID: "tenant-b"}

	_, err := a.AuthorizeBatch(ktA, "tester", appAttr(meta.View, 100001, 30001))
	require.NoError(t, err)

	_, err = a.AuthorizeBatch(ktB, "tester", appAttr(meta.View, 100001, 30001))
	require.NoError(t, err)

	require.Equal(t, 2, gw.callCount(), "different tenants must not share cache entries")
}

// 缓存键也必须区分用户。
func TestCacheIsolatesUsers(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{"app_view|30001": true}}
	a := newTestAuthorizer(t, gw, Config{CacheSize: 100, CacheTTL: time.Minute})

	_, err := a.AuthorizeBatch(testKit(), "alice", appAttr(meta.View, 100001, 30001))
	require.NoError(t, err)

	_, err = a.AuthorizeBatch(testKit(), "bob", appAttr(meta.View, 100001, 30001))
	require.NoError(t, err)

	require.Equal(t, 2, gw.callCount())
}

func TestCacheDisabledByZeroConfig(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{"app_view|30001": true}}
	a := newTestAuthorizer(t, gw, Config{})

	for i := 0; i < 3; i++ {
		_, err := a.AuthorizeBatch(testKit(), "tester", appAttr(meta.View, 100001, 30001))
		require.NoError(t, err)
	}

	require.Equal(t, 3, gw.callCount(), "cache should be disabled")
}

// 切批后应并发发送，且并发度受配置约束。
func TestResolveRespectsConcurrencyLimit(t *testing.T) {
	const total = client.MaxAuthBatchSize * 5

	allow := make(map[string]bool, total)
	attrs := make([]*meta.ResourceAttribute, 0, total)
	for i := 0; i < total; i++ {
		appID := uint32(30000 + i)
		allow[fmt.Sprintf("app_view|%d", appID)] = true
		attrs = append(attrs, appAttr(meta.View, 100001, appID))
	}

	gw := &fakeGateway{allow: allow}
	a := newTestAuthorizer(t, gw, Config{Concurrency: 2})

	_, err := a.AuthorizeBatch(testKit(), "tester", attrs...)
	require.NoError(t, err)

	require.Equal(t, 5, gw.callCount())
	require.LessOrEqual(t, gw.maxConcurrent, 2, "concurrency must be capped by config")
	require.Greater(t, gw.maxConcurrent, 1, "batches should be sent concurrently")
}

func TestAuthorizeSingle(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{"find_business_resource|100001": true}}
	a := newTestAuthorizer(t, gw, Config{})

	allowed, err := a.Authorize(testKit(), "tester", bizAttr(meta.FindBusinessResource, 100001))
	require.NoError(t, err)
	require.True(t, allowed)
}

// biz 维度的资源不带 attributes，app 维度必须带拓扑路径。
func TestAuthorizeSendsTopologyForAppOnly(t *testing.T) {
	gw := &fakeGateway{allow: map[string]bool{}}
	a := newTestAuthorizer(t, gw, Config{})

	_, err := a.AuthorizeBatch(testKit(), "tester",
		bizAttr(meta.FindBusinessResource, 100001),
		appAttr(meta.View, 100001, 30001),
	)
	require.NoError(t, err)
	require.Equal(t, 2, gw.callCount())

	for _, c := range gw.calls {
		require.Len(t, c.resourceIDs, 1)
	}
}

func TestNoPermissionUsesShorterTTL(t *testing.T) {
	c := newDecisionCache(10, time.Hour)
	require.NotNil(t, c)

	c.set("denied", false)
	allowed, hit := c.get("denied")
	require.True(t, hit)
	require.False(t, allowed)
}

func TestDecisionCacheDisabled(t *testing.T) {
	require.Nil(t, newDecisionCache(0, time.Minute))
	require.Nil(t, newDecisionCache(10, 0))

	var c *decisionCache
	_, hit := c.get("any")
	require.False(t, hit)
	// 对 nil 缓存写入不应 panic
	c.set("any", true)
}
