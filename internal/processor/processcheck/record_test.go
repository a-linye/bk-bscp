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

package processcheck

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// fakeExceptionStore 实现 dao.ProcessManagedException，记录调用以供断言。
type fakeExceptionStore struct {
	mu sync.Mutex

	// 注入返回值
	isExceptionRet map[uint32]bool                           // processInstanceID -> 是否异常
	latest         map[uint32]*table.ProcessManagedException // processInstanceID -> 最新记录
	updateErr      error

	// 调用记录
	created      []*table.ProcessManagedException
	updateCalls  []updateCall
	isExcCalls   int
	getLatestHit int
}

type updateCall struct {
	id     uint32
	status table.ProcessExceptionStatus
}

func newFakeStore() *fakeExceptionStore {
	return &fakeExceptionStore{
		isExceptionRet: map[uint32]bool{},
		latest:         map[uint32]*table.ProcessManagedException{},
	}
}

func (f *fakeExceptionStore) Create(_ *kit.Kit, m *table.ProcessManagedException) (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, m)
	return uint32(len(f.created)), nil
}

func (f *fakeExceptionStore) ListByProcessInstanceID(_ *kit.Kit, _, _ uint32) (
	[]*table.ProcessManagedException, error) {
	return nil, nil
}

func (f *fakeExceptionStore) GetLatestByProcessInstanceID(_ *kit.Kit, _, processInstanceID uint32) (
	*table.ProcessManagedException, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getLatestHit++
	return f.latest[processInstanceID], nil
}

func (f *fakeExceptionStore) IsException(_ *kit.Kit, _, processInstanceID uint32) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isExcCalls++
	return f.isExceptionRet[processInstanceID], nil
}

func (f *fakeExceptionStore) UpdateStatus(_ *kit.Kit, _, id uint32, status table.ProcessExceptionStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls = append(f.updateCalls, updateCall{id: id, status: status})
	return f.updateErr
}

func exceptionCheckResult() CheckResult {
	return CheckResult{
		ProcessInstanceID:  1,
		ProcessID:          11,
		HostID:             101,
		BizID:              sampleBizID,
		TenantID:           "t1",
		Verdict:            VerdictException,
		ErrorType:          table.ProcessExceptionExpectationMismatch,
		ErrorMsg:           "托管信息与预期不符, 差异字段: [startCmd]",
		HandlingSuggestion: suggestionMismatch,
		CheckedAt:          time.Now(),
	}
}

func TestApplyResult_ExceptionCreates(t *testing.T) {
	store := newFakeStore()
	r := exceptionCheckResult()

	if err := ApplyResult(kit.New(), store, r); err != nil {
		t.Fatalf("apply exception failed: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("want 1 created record, got %d", len(store.created))
	}
	m := store.created[0]
	if m.Spec.Status != table.ProcessExceptionStatusException {
		t.Fatalf("want status exception, got %s", m.Spec.Status)
	}
	if m.Spec.ErrorType != table.ProcessExceptionExpectationMismatch || m.Spec.ErrorMsg == "" ||
		m.Spec.HandlingSuggestion == "" || m.Spec.CheckedAt.IsZero() {
		t.Fatalf("spec fields incomplete: %+v", m.Spec)
	}
	if m.Attachment.HostID != 101 || m.Attachment.ProcessID != 11 ||
		m.Attachment.ProcessInstanceID != 1 || m.Attachment.BizID != sampleBizID || m.Attachment.TenantID != "t1" {
		t.Fatalf("attachment locator incomplete: %+v", m.Attachment)
	}
	if len(store.updateCalls) != 0 {
		t.Fatalf("exception should not update status")
	}
}

func TestApplyResult_PassRecovers(t *testing.T) {
	store := newFakeStore()
	store.isExceptionRet[1] = true
	store.latest[1] = &table.ProcessManagedException{ID: 77, Spec: &table.ProcessManagedExceptionSpec{
		Status: table.ProcessExceptionStatusException}}

	r := exceptionCheckResult()
	r.Verdict = VerdictPass

	if err := ApplyResult(kit.New(), store, r); err != nil {
		t.Fatalf("apply pass failed: %v", err)
	}
	if store.getLatestHit != 1 {
		t.Fatalf("want GetLatest called once, got %d", store.getLatestHit)
	}
	if len(store.updateCalls) != 1 || store.updateCalls[0].id != 77 ||
		store.updateCalls[0].status != table.ProcessExceptionStatusRecovered {
		t.Fatalf("want UpdateStatus(77, recovered), got %+v", store.updateCalls)
	}
	if len(store.created) != 0 {
		t.Fatalf("recovery should not create record")
	}
}

func TestApplyResult_PassNoExceptionNoop(t *testing.T) {
	store := newFakeStore()
	store.isExceptionRet[1] = false // 无记录或已 recovered

	r := exceptionCheckResult()
	r.Verdict = VerdictPass

	if err := ApplyResult(kit.New(), store, r); err != nil {
		t.Fatalf("apply pass(noop) failed: %v", err)
	}
	if len(store.created) != 0 || len(store.updateCalls) != 0 || store.getLatestHit != 0 {
		t.Fatalf("non-exception pass must be no-op, created=%d update=%d getLatest=%d",
			len(store.created), len(store.updateCalls), store.getLatestHit)
	}
}

func TestApplyResult_PassUpdateError(t *testing.T) {
	store := newFakeStore()
	store.isExceptionRet[1] = true
	store.latest[1] = &table.ProcessManagedException{ID: 5, Spec: &table.ProcessManagedExceptionSpec{
		Status: table.ProcessExceptionStatusException}}
	store.updateErr = errors.New("db down")

	r := exceptionCheckResult()
	r.Verdict = VerdictPass

	if err := ApplyResult(kit.New(), store, r); err == nil {
		t.Fatal("want UpdateStatus error propagated, got nil")
	}
}

func TestApplyResult_SkipNoop(t *testing.T) {
	store := newFakeStore()
	r := exceptionCheckResult()
	r.Verdict = VerdictSkip

	if err := ApplyResult(kit.New(), store, r); err != nil {
		t.Fatalf("apply skip failed: %v", err)
	}
	if len(store.created) != 0 || len(store.updateCalls) != 0 || store.isExcCalls != 0 {
		t.Fatalf("skip must be no-op")
	}
}
