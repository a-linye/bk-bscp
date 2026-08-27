/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
    10| * limitations under the License.
*/

package cmdb

import (
	"context"
	"reflect"
	"testing"

	"github.com/TencentBlueKing/bk-bscp/internal/components/bkcmdb"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

type findModuleStubCMDB struct {
	bkcmdb.Service
	calls [][]int
}

func (m *findModuleStubCMDB) FindModuleBatch(_ context.Context, req *bkcmdb.ModuleReq) ([]*bkcmdb.ModuleInfo, error) {
	ids := append([]int(nil), req.BkIDs...)
	m.calls = append(m.calls, ids)
	out := make([]*bkcmdb.ModuleInfo, 0, len(req.BkIDs))
	for _, id := range req.BkIDs {
		out = append(out, &bkcmdb.ModuleInfo{BkModuleID: id, ServiceTemplateID: id * 10})
	}
	return out, nil
}

func newProcessWithModule(moduleID uint32) *table.Process {
	return &table.Process{
		Attachment: &table.ProcessAttachment{ModuleID: moduleID},
	}
}

func TestFillServiceTemplateIDDedupesAndSkipsInvalid(t *testing.T) {
	stub := &findModuleStubCMDB{}
	s := &syncCMDBService{bizID: 3, svc: stub}

	procs := []*table.Process{
		newProcessWithModule(10),
		newProcessWithModule(10),
		newProcessWithModule(0),
		newProcessWithModule(20),
		nil,
		{Attachment: nil},
	}

	if err := s.fillServiceTemplateID(context.Background(), procs); err != nil {
		t.Fatalf("fillServiceTemplateID failed: %v", err)
	}

	if len(stub.calls) != 1 {
		t.Fatalf("FindModuleBatch calls = %d, want 1", len(stub.calls))
	}
	if !reflect.DeepEqual(stub.calls[0], []int{10, 20}) {
		t.Fatalf("requested module ids = %v, want [10 20]", stub.calls[0])
	}
	if procs[0].Attachment.ServiceTemplateID != 100 {
		t.Fatalf("proc0 service_template_id = %d, want 100", procs[0].Attachment.ServiceTemplateID)
	}
	if procs[1].Attachment.ServiceTemplateID != 100 {
		t.Fatalf("proc1 service_template_id = %d, want 100", procs[1].Attachment.ServiceTemplateID)
	}
	if procs[3].Attachment.ServiceTemplateID != 200 {
		t.Fatalf("proc3 service_template_id = %d, want 200", procs[3].Attachment.ServiceTemplateID)
	}
}

func TestFillServiceTemplateIDBatchesByLimit(t *testing.T) {
	stub := &findModuleStubCMDB{}
	s := &syncCMDBService{bizID: 3, svc: stub}

	procs := make([]*table.Process, 0, findModuleBatchLimit+1)
	for i := 1; i <= findModuleBatchLimit+1; i++ {
		procs = append(procs, newProcessWithModule(uint32(i)))
	}

	if err := s.fillServiceTemplateID(context.Background(), procs); err != nil {
		t.Fatalf("fillServiceTemplateID failed: %v", err)
	}

	if len(stub.calls) != 2 {
		t.Fatalf("FindModuleBatch calls = %d, want 2", len(stub.calls))
	}
	if len(stub.calls[0]) != findModuleBatchLimit {
		t.Fatalf("first batch size = %d, want %d", len(stub.calls[0]), findModuleBatchLimit)
	}
	if len(stub.calls[1]) != 1 {
		t.Fatalf("second batch size = %d, want 1", len(stub.calls[1]))
	}
}

func TestFillServiceTemplateIDEmpty(t *testing.T) {
	stub := &findModuleStubCMDB{}
	s := &syncCMDBService{bizID: 3, svc: stub}

	if err := s.fillServiceTemplateID(context.Background(), nil); err != nil {
		t.Fatalf("fillServiceTemplateID failed: %v", err)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("FindModuleBatch calls = %d, want 0", len(stub.calls))
	}
}

func TestUniquePositiveInts(t *testing.T) {
	got := uniquePositiveInts([]int{0, 10, 10, -1, 20, 0, 20})
	if !reflect.DeepEqual(got, []int{10, 20}) {
		t.Fatalf("uniquePositiveInts = %v, want [10 20]", got)
	}
}
