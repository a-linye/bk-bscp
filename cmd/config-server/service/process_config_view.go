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

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
	pbcs "github.com/TencentBlueKing/bk-bscp/pkg/protocol/config-server"
	pbds "github.com/TencentBlueKing/bk-bscp/pkg/protocol/data-service"
)

// UpsertBizProcessConfigView creates or updates a biz's process config view setting.
func (s *Service) UpsertBizProcessConfigView(ctx context.Context,
	req *pbcs.UpsertBizProcessConfigViewReq) (*pbcs.UpsertBizProcessConfigViewResp, error) {

	grpcKit := kit.FromGrpcContext(ctx)

	if _, err := s.client.DS.UpsertBizProcessConfigView(grpcKit.RpcCtx(), &pbds.UpsertBizProcessConfigViewReq{
		BizId:   req.BizId,
		Enabled: req.Enabled,
	}); err != nil {
		logs.Errorf("upsert biz process config view failed, bizID: %d, err: %v, rid: %s",
			req.BizId, err, grpcKit.Rid)
		return nil, err
	}

	return &pbcs.UpsertBizProcessConfigViewResp{}, nil
}

// DeleteBizProcessConfigView removes a biz's process config view setting.
func (s *Service) DeleteBizProcessConfigView(ctx context.Context,
	req *pbcs.DeleteBizProcessConfigViewReq) (*pbcs.DeleteBizProcessConfigViewResp, error) {

	grpcKit := kit.FromGrpcContext(ctx)

	if _, err := s.client.DS.DeleteBizProcessConfigView(grpcKit.RpcCtx(), &pbds.DeleteBizProcessConfigViewReq{
		BizId: req.BizId,
	}); err != nil {
		logs.Errorf("delete biz process config view failed, bizID: %d, err: %v, rid: %s",
			req.BizId, err, grpcKit.Rid)
		return nil, err
	}

	return &pbcs.DeleteBizProcessConfigViewResp{}, nil
}

// ListBizProcessConfigView lists all configured biz process config view entries.
func (s *Service) ListBizProcessConfigView(ctx context.Context,
	req *pbcs.ListBizProcessConfigViewReq) (*pbcs.ListBizProcessConfigViewResp, error) {

	grpcKit := kit.FromGrpcContext(ctx)

	resp, err := s.client.DS.ListBizProcessConfigView(grpcKit.RpcCtx(), &pbds.ListBizProcessConfigViewReq{})
	if err != nil {
		logs.Errorf("list biz process config view failed, err: %v, rid: %s", err, grpcKit.Rid)
		return nil, err
	}

	items := make([]*pbcs.BizProcessConfigViewItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, &pbcs.BizProcessConfigViewItem{
			BizId:   item.BizId,
			Enabled: item.Enabled,
		})
	}

	return &pbcs.ListBizProcessConfigViewResp{Items: items}, nil
}
