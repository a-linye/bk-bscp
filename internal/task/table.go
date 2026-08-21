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
	"time"

	"gorm.io/gorm"
)

// 以下表结构复制自 bcs-common/common/task/stores/mysql 的 TaskRecord、StepRecord，
// 唯一的差异是把存放任务负载的 common_payload、payload 由 text 放大为 mediumtext。
// 原因: 配置内容会作为任务快照写入负载，JSON 序列化时 XML 的尖括号会被转义成 6 字节的
// \uXXXX，一份十几 KB 的配置就能超过 text 的 65535 字节上限，而 MySQL 非严格模式下
// 超长写入是静默截断，负载 JSON 会变成不完整的串，之后任何读取都会解析失败。
//
// 这两个结构体只用于 EnsureTable 建表与列变更，任务的读写仍由框架自身的模型完成，
// 因此字段名、类型、索引名必须与框架保持一致，框架升级新增字段时需同步到此处。

// BaseModel 对齐框架 BaseModel，为 CreatedAt 添加索引
// 必须是导出类型，gorm 不会解析非导出的嵌入结构体
type BaseModel struct {
	gorm.Model
	CreatedAt time.Time `gorm:"index"`
}

// Record 任务记录，对应表 task_records
// 对应框架的 TaskRecord，此处不带 Task 前缀是因为会与包名重复（revive stutter）
type Record struct {
	BaseModel
	TaskID              string            `gorm:"type:varchar(191);uniqueIndex:idx_task_id"`
	TaskType            string            `gorm:"type:varchar(191);index:idx_task_type"`
	TaskIndex           string            `gorm:"type:varchar(191);index:idx_task_index"`
	TaskIndexType       string            `gorm:"type:varchar(191);index:idx_task_index"`
	TaskName            string            `gorm:"type:varchar(255)"`
	CurrentStep         string            `gorm:"type:varchar(255)"`
	StepSequence        []string          `gorm:"type:text;serializer:json"`
	CallbackName        string            `gorm:"type:varchar(255)"`
	CallbackResult      string            `gorm:"type:varchar(191)"`
	CallbackMessage     string            `gorm:"type:text"`
	CommonParams        map[string]string `gorm:"type:text;serializer:json"`
	CommonPayload       string            `gorm:"type:mediumtext"`
	Status              string            `gorm:"type:varchar(191);index:idx_status"`
	Message             string            `gorm:"type:text"`
	ExecutionTime       uint32
	MaxExecutionSeconds uint32
	Start               time.Time
	End                 time.Time
	Creator             string `gorm:"type:varchar(255)"`
	Updater             string `gorm:"type:varchar(255)"`
}

// TableName ..
func (t *Record) TableName() string {
	return "task_records"
}

// StepRecord 步骤记录，对应表 task_step_records
type StepRecord struct {
	gorm.Model
	TaskID              string            `gorm:"type:varchar(191);uniqueIndex:idx_task_id_step_name"`
	Name                string            `gorm:"type:varchar(191);uniqueIndex:idx_task_id_step_name"`
	Alias               string            `gorm:"type:varchar(255)"`
	Executor            string            `gorm:"type:varchar(255)"`
	Params              map[string]string `gorm:"type:text;serializer:json"`
	Payload             string            `gorm:"type:mediumtext"`
	Status              string            `gorm:"type:varchar(255)"`
	Message             string            `gorm:"type:text"`
	ETA                 *time.Time
	SkipOnFailed        bool
	RetryCount          uint32
	MaxRetries          uint32
	ExecutionTime       uint32
	MaxExecutionSeconds uint32
	Start               time.Time
	End                 time.Time
}

// TableName ..
func (t *StepRecord) TableName() string {
	return "task_step_records"
}
