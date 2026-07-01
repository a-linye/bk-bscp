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

package table

import (
	"reflect"
	"testing"
)

func TestProcessExceptionErrorTypeValidate(t *testing.T) {
	valid := []ProcessExceptionErrorType{
		ProcessExceptionParsingFailed,
		ProcessExceptionAgentException,
		ProcessExceptionIllegalValueKey,
		ProcessExceptionExpectationMismatch,
		ProcessExceptionOther,
	}
	for _, v := range valid {
		if err := v.Validate(); err != nil {
			t.Errorf("error type %q should be valid, got err: %v", v, err)
		}
	}

	if err := ProcessExceptionErrorType("INVALID").Validate(); err == nil {
		t.Errorf("invalid error type should return error")
	}
}

func TestProcessExceptionStatusValidate(t *testing.T) {
	valid := []ProcessExceptionStatus{
		ProcessExceptionStatusException,
		ProcessExceptionStatusRecovered,
	}
	for _, v := range valid {
		if err := v.Validate(); err != nil {
			t.Errorf("status %q should be valid, got err: %v", v, err)
		}
	}

	if err := ProcessExceptionStatus("invalid").Validate(); err == nil {
		t.Errorf("invalid status should return error")
	}
}

func TestProcessManagedExceptionTableName(t *testing.T) {
	pme := new(ProcessManagedException)
	if pme.TableName() != ProcessManagedExceptionsTable {
		t.Errorf("table name mismatch, want %q, got %q", ProcessManagedExceptionsTable, pme.TableName())
	}
}

// TestProcessManagedExceptionAttachmentTenantField 校验 Attachment 含 TenantID 字段：
// set_tenant_id 回调按字段名 LookUpField("TenantID") 注入/过滤，字段名缺失会使多租户隔离失效。
func TestProcessManagedExceptionAttachmentTenantField(t *testing.T) {
	field, ok := reflect.TypeOf(ProcessManagedExceptionAttachment{}).FieldByName("TenantID")
	if !ok {
		t.Fatalf("ProcessManagedExceptionAttachment must have field TenantID")
	}
	if field.Type.Kind() != reflect.String {
		t.Errorf("TenantID field must be string, got %s", field.Type.Kind())
	}
}
