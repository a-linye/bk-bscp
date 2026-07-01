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
	"errors"
	"time"
)

// ProcessManagedException 托管异常记录：一条记录对应"某进程实例某次检查的异常结论"。
// 检查侧每次发现异常追加写入一条，非覆盖；历史明细全部保留。
type ProcessManagedException struct {
	ID         uint32                             `json:"id" gorm:"primaryKey"`
	Attachment *ProcessManagedExceptionAttachment `json:"attachment" gorm:"embedded"`
	Spec       *ProcessManagedExceptionSpec       `json:"spec" gorm:"embedded"`
	Revision   *Revision                          `json:"revision" gorm:"embedded"`
}

// TableName is the process managed exception's database table name.
func (p *ProcessManagedException) TableName() Name {
	return ProcessManagedExceptionsTable
}

// ProcessManagedExceptionSpec 业务字段
type ProcessManagedExceptionSpec struct {
	ErrorType          ProcessExceptionErrorType `json:"error_type" gorm:"column:error_type"`                   // 异常类型枚举
	ErrorMsg           string                    `json:"error_msg" gorm:"column:error_msg"`                     // 异常描述（含具体差异信息，长度不定）
	HandlingSuggestion string                    `json:"handling_suggestion" gorm:"column:handling_suggestion"` // 处理建议
	Status             ProcessExceptionStatus    `json:"status" gorm:"column:status"`                           // 记录状态枚举
	CheckedAt          time.Time                 `json:"checked_at" gorm:"column:checked_at"`                   // 检查时间，由检查侧写入时传入
}

// ProcessManagedExceptionAttachment 定位字段，冗余存储免 join。
type ProcessManagedExceptionAttachment struct {
	// TenantID 字段名必须为 TenantID：set_tenant_id 回调按此名 LookUpField 自动注入/过滤租户。
	TenantID string `json:"tenant_id" gorm:"column:tenant_id"`
	BizID    uint32 `json:"biz_id" gorm:"column:biz_id"`
	// HostID process_instances 表不含主机 ID，取自 ProcessAttachment 冗余存储。
	HostID            uint32 `json:"host_id" gorm:"column:host_id"`
	ProcessID         uint32 `json:"process_id" gorm:"column:process_id"`
	ProcessInstanceID uint32 `json:"process_instance_id" gorm:"column:process_instance_id"`
}

// ProcessExceptionErrorType 异常类型，对标 gsekit ProcessCheckManager.ErrorType。
type ProcessExceptionErrorType string

const (
	// ProcessExceptionParsingFailed 解析失败
	ProcessExceptionParsingFailed ProcessExceptionErrorType = "PARSING_FAILED"
	// ProcessExceptionAgentException agent 异常
	ProcessExceptionAgentException ProcessExceptionErrorType = "AGENT_EXCEPTION"
	// ProcessExceptionIllegalValueKey 非法 valuekey
	ProcessExceptionIllegalValueKey ProcessExceptionErrorType = "ILLEGAL_VALUE_KEY"
	// ProcessExceptionExpectationMismatch 配置不符（已托管无信息/未托管有信息/属性差异）
	ProcessExceptionExpectationMismatch ProcessExceptionErrorType = "EXPECTATION_MISMATCH"
	// ProcessExceptionOther 其他
	ProcessExceptionOther ProcessExceptionErrorType = "OTHER"
)

// String get string value of process exception error type.
func (t ProcessExceptionErrorType) String() string {
	return string(t)
}

// Validate validate process exception error type is valid or not.
func (t ProcessExceptionErrorType) Validate() error {
	switch t {
	case ProcessExceptionParsingFailed, ProcessExceptionAgentException, ProcessExceptionIllegalValueKey,
		ProcessExceptionExpectationMismatch, ProcessExceptionOther:
		return nil
	default:
		return errors.New("invalid process exception error type")
	}
}

// ProcessExceptionStatus 记录状态：异常 / 已恢复。
type ProcessExceptionStatus string

const (
	// ProcessExceptionStatusException 异常
	ProcessExceptionStatusException ProcessExceptionStatus = "exception"
	// ProcessExceptionStatusRecovered 已恢复
	ProcessExceptionStatusRecovered ProcessExceptionStatus = "recovered"
)

// String get string value of process exception status.
func (s ProcessExceptionStatus) String() string {
	return string(s)
}

// Validate validate process exception status is valid or not.
func (s ProcessExceptionStatus) Validate() error {
	switch s {
	case ProcessExceptionStatusException, ProcessExceptionStatusRecovered:
		return nil
	default:
		return errors.New("invalid process exception status")
	}
}
