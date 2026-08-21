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

package task

import (
	"sync"
	"testing"

	mysqlstore "github.com/Tencent/bk-bcs/bcs-common/common/task/stores/mysql"
	"gorm.io/gorm/schema"
)

// payloadColumnOverrides 本地表结构与框架表结构允许存在的差异:
// 只有存放任务负载的列由 text 放大为 mediumtext，其余必须完全一致，
// 否则 EnsureTable 的 AutoMigrate 会改动到不该改的列。
var payloadColumnOverrides = map[string]map[string]string{
	"task_records":      {"common_payload": "mediumtext"},
	"task_step_records": {"payload": "mediumtext"},
}

func parseSchema(t *testing.T, dst any) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(dst, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse schema failed: %v", err)
	}
	return s
}

// assertSchemaAligned 校验本地表结构与框架表结构的列、列类型、索引保持一致
func assertSchemaAligned(t *testing.T, local, upstream any) {
	t.Helper()

	localSchema, upstreamSchema := parseSchema(t, local), parseSchema(t, upstream)

	if localSchema.Table != upstreamSchema.Table {
		t.Fatalf("table name mismatch: local %s, upstream %s", localSchema.Table, upstreamSchema.Table)
	}
	overrides := payloadColumnOverrides[localSchema.Table]

	localTypes := columnTypes(localSchema)
	upstreamTypes := columnTypes(upstreamSchema)

	for column, upstreamType := range upstreamTypes {
		localType, ok := localTypes[column]
		if !ok {
			t.Errorf("%s: column %s missing in local schema, sync it from the framework",
				localSchema.Table, column)
			continue
		}
		want := upstreamType
		if override, isOverridden := overrides[column]; isOverridden {
			want = override
		}
		if localType != want {
			t.Errorf("%s.%s: type is %q, want %q", localSchema.Table, column, localType, want)
		}
	}

	for column := range localTypes {
		if _, ok := upstreamTypes[column]; !ok {
			t.Errorf("%s: column %s does not exist in the framework schema", localSchema.Table, column)
		}
	}

	assertIndexesAligned(t, localSchema, upstreamSchema)
}

func columnTypes(s *schema.Schema) map[string]string {
	types := make(map[string]string, len(s.Fields))
	for _, field := range s.Fields {
		if field.DBName == "" {
			continue
		}
		types[field.DBName] = field.TagSettings["TYPE"]
	}
	return types
}

func assertIndexesAligned(t *testing.T, localSchema, upstreamSchema *schema.Schema) {
	t.Helper()

	localIndexes := indexSignatures(localSchema)
	upstreamIndexes := indexSignatures(upstreamSchema)

	for name, upstreamSignature := range upstreamIndexes {
		localSignature, ok := localIndexes[name]
		if !ok {
			t.Errorf("%s: index %s missing in local schema", localSchema.Table, name)
			continue
		}
		if localSignature != upstreamSignature {
			t.Errorf("%s: index %s is %q, want %q", localSchema.Table, name, localSignature, upstreamSignature)
		}
	}

	for name := range localIndexes {
		if _, ok := upstreamIndexes[name]; !ok {
			t.Errorf("%s: index %s does not exist in the framework schema", localSchema.Table, name)
		}
	}
}

// indexSignatures 以 "类型:列1,列2" 的形式描述每个索引，便于直接比较
func indexSignatures(s *schema.Schema) map[string]string {
	signatures := make(map[string]string)
	for _, index := range s.ParseIndexes() {
		signature := index.Class
		for _, field := range index.Fields {
			signature += ":" + field.DBName
		}
		signatures[index.Name] = signature
	}
	return signatures
}

// TestTaskRecordAlignedWithFramework 本地 task_records 定义须与框架一致，仅负载列放大
func TestTaskRecordAlignedWithFramework(t *testing.T) {
	assertSchemaAligned(t, &Record{}, &mysqlstore.TaskRecord{})
}

// TestStepRecordAlignedWithFramework 本地 task_step_records 定义须与框架一致，仅负载列放大
func TestStepRecordAlignedWithFramework(t *testing.T) {
	assertSchemaAligned(t, &StepRecord{}, &mysqlstore.StepRecord{})
}

// TestPayloadColumnsUseMediumText 负载列必须是 mediumtext，防止被改回 text 后又出现静默截断
func TestPayloadColumnsUseMediumText(t *testing.T) {
	for _, dst := range []any{&Record{}, &StepRecord{}} {
		s := parseSchema(t, dst)
		for column, want := range payloadColumnOverrides[s.Table] {
			if got := columnTypes(s)[column]; got != want {
				t.Errorf("%s.%s: type is %q, want %q", s.Table, column, got, want)
			}
		}
	}
}
