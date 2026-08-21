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

package event

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TencentBlueKing/bk-bscp/cmd/cache-service/service/cache/client"
	"github.com/TencentBlueKing/bk-bscp/internal/criteria/constant"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	pbclient "github.com/TencentBlueKing/bk-bscp/pkg/protocol/core/client"
	"github.com/TencentBlueKing/bk-bscp/pkg/runtime/jsoni"
	sfs "github.com/TencentBlueKing/bk-bscp/pkg/sf-share"
)

// tenantResolverStub 只实现租户反查，其余方法继承自嵌入的 nil 接口（测试中不会被调用）
type tenantResolverStub struct {
	client.Interface

	tenants map[uint32]string
	err     error
	calls   int
}

func (s *tenantResolverStub) GetTenantIDByBiz(_ *kit.Kit, bizID uint32, _ bool) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	return s.tenants[bizID], nil
}

// setMultiTenantMode 设置全局租户开关，测试结束后还原
func setMultiTenantMode(t *testing.T, enabled bool) {
	t.Helper()
	settings := cc.GlobalSettings{}
	settings.FeatureFlags.EnableMultiTenantMode = enabled
	cc.SetG(settings)
	t.Cleanup(func() { cc.SetG(cc.GlobalSettings{}) })
}

func clientsOfBiz(bizIDs ...uint32) []*pbclient.Client {
	items := make([]*pbclient.Client, 0, len(bizIDs))
	for _, bizID := range bizIDs {
		items = append(items, &pbclient.Client{Attachment: &pbclient.ClientAttachment{BizId: bizID}})
	}
	return items
}

// payload 已带租户时直接沿用，不再按 biz_id 反查（反查缓存陈旧会解析出错误租户）
func TestBuildBizTenantMapPrefersPayloadTenant(t *testing.T) {
	setMultiTenantMode(t, true)

	stub := &tenantResolverStub{tenants: map[uint32]string{15: constant.DefaultTenantID}}
	cm := &ClientMetric{op: stub}

	got, err := cm.buildBizTenantMap(kit.New(), clientsOfBiz(15, 15), map[uint32]string{15: "system"})

	require.NoError(t, err)
	assert.Equal(t, map[uint32]string{15: "system"}, got)
	assert.Zero(t, stub.calls, "payload 已带租户时不应触发反查")
}

// 旧版本 feed-server 入队的数据不带租户，多租户模式下回退到反查
func TestBuildBizTenantMapFallsBackToLookup(t *testing.T) {
	setMultiTenantMode(t, true)

	stub := &tenantResolverStub{tenants: map[uint32]string{15: "system"}}
	cm := &ClientMetric{op: stub}

	got, err := cm.buildBizTenantMap(kit.New(), clientsOfBiz(15, 15), nil)

	require.NoError(t, err)
	assert.Equal(t, map[uint32]string{15: "system"}, got)
	assert.Equal(t, 1, stub.calls, "同一业务只反查一次")
}

// 多租户模式下反查不出租户时必须报错，不能静默落到 default 租户
func TestBuildBizTenantMapRejectsEmptyTenant(t *testing.T) {
	setMultiTenantMode(t, true)

	cm := &ClientMetric{op: &tenantResolverStub{tenants: map[uint32]string{}}}

	_, err := cm.buildBizTenantMap(kit.New(), clientsOfBiz(15), nil)

	require.Error(t, err)
}

func TestBuildBizTenantMapPropagatesLookupError(t *testing.T) {
	setMultiTenantMode(t, true)

	cm := &ClientMetric{op: &tenantResolverStub{err: errors.New("cache unavailable")}}

	_, err := cm.buildBizTenantMap(kit.New(), clientsOfBiz(15), nil)

	require.Error(t, err)
}

// 单租户模式下 payload 仍然优先，避免两侧租户开关不一致时把数据写到 default 租户
func TestBuildBizTenantMapSingleTenant(t *testing.T) {
	setMultiTenantMode(t, false)

	stub := &tenantResolverStub{}
	cm := &ClientMetric{op: stub}

	got, err := cm.buildBizTenantMap(kit.New(), clientsOfBiz(15, 16), map[uint32]string{15: "system"})

	require.NoError(t, err)
	assert.Equal(t, map[uint32]string{15: "system", 16: constant.DefaultTenantID}, got)
	assert.Zero(t, stub.calls)
}

// 队列消息体的租户字段必须能跨 feed-server / cache-service 序列化往返，
// 且旧版本不带该字段的数据仍可正常解析
func TestClientMetricDataTenantIDRoundTrip(t *testing.T) {
	raw, err := jsoni.Marshal(&sfs.ClientMetricData{
		MessagingType: uint32(sfs.Heartbeat),
		Payload:       []byte(`{}`),
		TenantID:      "system",
	})
	require.NoError(t, err)

	var decoded sfs.ClientMetricData
	require.NoError(t, jsoni.Unmarshal(raw, &decoded))
	assert.Equal(t, "system", decoded.TenantID)

	var legacy sfs.ClientMetricData
	require.NoError(t, jsoni.Unmarshal([]byte(`{"MessagingType":9,"Payload":"e30="}`), &legacy))
	assert.Empty(t, legacy.TenantID)
}
