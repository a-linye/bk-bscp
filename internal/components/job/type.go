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

package job

// ScopeType 资源范围类型
type ScopeType string

// 资源范围类型。可选值: biz - 业务，biz_set - 业务集
const (
	ScopeTypeBiz    ScopeType = "biz"
	ScopeTypeBizSet ScopeType = "biz_set"
)

// PushConfigFileReq 分发配置文件请求
type PushConfigFileReq struct {
	BkScopeType    ScopeType     `json:"bk_scope_type"`    // 资源范围类型
	BkScopeID      string        `json:"bk_scope_id"`      // 资源范围ID, 与bk_scope_type对应, 表示业务ID或者业务集ID
	TaskName       string        `json:"task_name"`        // 自定义作业名称，非必传
	Account        string        `json:"account"`          // 执行账号
	FileTargetPath string        `json:"file_target_path"` // 文件目标路径
	FileList       []FileItem    `json:"file_list"`        // 文件列表
	TargetServer   *TargetServer `json:"target_server"`    // 目标服务器
}

// FileItem 文件项
type FileItem struct {
	FileName string `json:"file_name"` // 文件名
	Content  string `json:"content"`   // 文件内容（base64编码）
}

// TargetServer 目标服务器
type TargetServer struct {
	DynamicGroupList []DynamicGroup `json:"dynamic_group_list"` // 动态分组列表
	HostIDList       []uint32       `json:"host_id_list"`       // 主机ID列表
	TopoNodeList     []TopoNode     `json:"topo_node_list"`     // 拓扑节点列表
}

// DynamicGroup 动态分组
type DynamicGroup struct {
	ID string `json:"id"` // CMDB动态分组ID
}

// NodeType 动态topo节点类型
type NodeType string

// 动态topo节点类型，对应CMDB API 中的 bk_obj_id,比如"module","set"
const (
	NodeTypeBiz    NodeType = "biz"
	NodeTypeSet    NodeType = "set"
	NodeTypeModule NodeType = "module"
)

// TopoNode 拓扑节点
type TopoNode struct {
	ID       uint64   `json:"id"`        // 节点ID
	NodeType NodeType `json:"node_type"` // 节点类型
}

// PushConfigFileResp 分发配置文件响应
type PushConfigFileResp struct {
	JobInstanceID   string `json:"job_instance_id"`   // 作业实例ID
	JobInstanceName string `json:"job_instance_name"` // 作业实例名称
	StepInstanceID  string `json:"step_instance_id"`  // 步骤实例ID
}

// GetJobInstanceStatusReq 查询作业执行状态请求
type GetJobInstanceStatusReq struct {
	BkScopeType    ScopeType `json:"bk_scope_type"`    // 资源范围类型
	BkScopeID      string    `json:"bk_scope_id"`      // 资源范围ID
	JobInstanceID  uint64    `json:"job_instance_id"`  // 作业实例ID
	ReturnIPResult bool      `json:"return_ip_result"` // 是否返回每个ip上的任务详情，对应返回结果中的step_ip_result_list。默认值为false。
}

// GetJobInstanceStatusResp 查询作业执行状态响应
type GetJobInstanceStatusResp struct {
	Finished         bool            `json:"finished"`           // 作业是否结束
	JobInstance      *JobInstance    `json:"job_instance"`       // 作业实例基本信息
	StepInstanceList []*StepInstance `json:"step_instance_list"` // 作业步骤列表
}

// JobStatus 作业状态
type JobStatus int

const (
	JobStatusNotStarted       JobStatus = 1  // 未执行
	JobStatusRunning          JobStatus = 2  // 正在执行
	JobStatusSuccess          JobStatus = 3  // 执行成功
	JobStatusFailed           JobStatus = 4  // 执行失败
	JobStatusSkipped          JobStatus = 5  // 跳过
	JobStatusIgnored          JobStatus = 6  // 忽略错误
	JobStatusWaiting          JobStatus = 7  // 等待用户
	JobStatusManualEnd        JobStatus = 8  // 手动结束
	JobStatusException        JobStatus = 9  // 状态异常
	JobStatusForceStop        JobStatus = 10 // 步骤强制终止中
	JobStatusForceStopSuccess JobStatus = 11 // 步骤强制终止成功
)

const (
	JobStatusUnknownStr          = "未知状态"
	JobStatusNotStartedStr       = "未执行"
	JobStatusRunningStr          = "正在执行"
	JobStatusSuccessStr          = "执行成功"
	JobStatusFailedStr           = "执行失败"
	JobStatusSkippedStr          = "跳过"
	JobStatusIgnoredStr          = "忽略错误"
	JobStatusWaitingStr          = "等待用户"
	JobStatusManualEndStr        = "手动结束"
	JobStatusExceptionStr        = "状态异常"
	JobStatusForceStopStr        = "步骤强制终止中"
	JobStatusForceStopSuccessStr = "步骤强制终止成功"
)

// String get string value of job status
func (j JobStatus) String() string {
	statusMap := map[JobStatus]string{
		JobStatusNotStarted:       JobStatusNotStartedStr,
		JobStatusRunning:          JobStatusRunningStr,
		JobStatusSuccess:          JobStatusSuccessStr,
		JobStatusFailed:           JobStatusFailedStr,
		JobStatusSkipped:          JobStatusSkippedStr,
		JobStatusIgnored:          JobStatusIgnoredStr,
		JobStatusWaiting:          JobStatusWaitingStr,
		JobStatusManualEnd:        JobStatusManualEndStr,
		JobStatusException:        JobStatusExceptionStr,
		JobStatusForceStop:        JobStatusForceStopStr,
		JobStatusForceStopSuccess: JobStatusForceStopSuccessStr,
	}
	if str, ok := statusMap[j]; ok {
		return str
	}
	return JobStatusUnknownStr
}

// JobInstance 作业实例信息
type JobInstance struct {
	JobInstanceID uint64    `json:"job_instance_id"` // 作业实例ID
	BkScopeType   string    `json:"bk_scope_type"`   // 资源范围类型
	BkScopeID     string    `json:"bk_scope_id"`     // 资源范围ID
	Name          string    `json:"name"`            // 作业实例名称
	CreateTime    int64     `json:"create_time"`     // 作业创建时间，Unix时间戳，单位毫秒
	Status        JobStatus `json:"status"`          // 作业状态
	StartTime     int64     `json:"start_time"`      // 开始执行时间，Unix时间戳，单位毫秒
	EndTime       int64     `json:"end_time"`        // 执行结束时间，Unix时间戳，单位毫秒
	TotalTime     int64     `json:"total_time"`      // 总耗时（毫秒）
}

type StepStatus int

const (
	StepStatusNotStarted       StepStatus = 1  // 未执行
	StepStatusRunning          StepStatus = 2  // 正在执行
	StepStatusSuccess          StepStatus = 3  // 执行成功
	StepStatusFailed           StepStatus = 4  // 执行失败
	StepStatusSkipped          StepStatus = 5  // 跳过
	StepStatusIgnored          StepStatus = 6  // 忽略错误
	StepStatusWaiting          StepStatus = 7  // 等待用户
	StepStatusManualEnd        StepStatus = 8  // 手动结束
	StepStatusException        StepStatus = 9  // 状态异常
	StepStatusForceStop        StepStatus = 10 // 步骤强制终止中
	StepStatusForceStopSuccess StepStatus = 11 // 步骤强制终止成功
	StepStatusForceStopFailed  StepStatus = 12 // 步骤强制终止失败
)
const (
	StepStatusNotStartedStr       = "未执行"
	StepStatusRunningStr          = "正在执行"
	StepStatusSuccessStr          = "执行成功"
	StepStatusFailedStr           = "执行失败"
	StepStatusSkippedStr          = "跳过"
	StepStatusIgnoredStr          = "忽略错误"
	StepStatusWaitingStr          = "等待用户"
	StepStatusManualEndStr        = "手动结束"
	StepStatusExceptionStr        = "状态异常"
	StepStatusForceStopStr        = "步骤强制终止中"
	StepStatusForceStopSuccessStr = "步骤强制终止成功"
	StepStatusForceStopFailedStr  = "步骤强制终止失败"
)

// String get string value of step status
func (s StepStatus) String() string {
	statusMap := map[StepStatus]string{
		StepStatusNotStarted:       StepStatusNotStartedStr,
		StepStatusRunning:          StepStatusRunningStr,
		StepStatusSuccess:          StepStatusSuccessStr,
		StepStatusFailed:           StepStatusFailedStr,
		StepStatusSkipped:          StepStatusSkippedStr,
		StepStatusIgnored:          StepStatusIgnoredStr,
		StepStatusWaiting:          StepStatusWaitingStr,
		StepStatusManualEnd:        StepStatusManualEndStr,
		StepStatusException:        StepStatusExceptionStr,
		StepStatusForceStop:        StepStatusForceStopStr,
		StepStatusForceStopSuccess: StepStatusForceStopSuccessStr,
		StepStatusForceStopFailed:  StepStatusForceStopFailedStr,
	}
	if str, ok := statusMap[s]; ok {
		return str
	}
	return StepStatusNotStartedStr
}

// StepType 步骤类型
type StepType int

const (
	StepTypeScript StepType = 1 // 脚本步骤
	StepTypeFile   StepType = 2 // 文件步骤
	StepTypeSQL    StepType = 4 // SQL步骤
)

// StepInstance 步骤实例信息
type StepInstance struct {
	Status           StepStatus      `json:"status"`              // 步骤状态
	TotalTime        int64           `json:"total_time"`          // 总耗时，单位毫秒
	Name             string          `json:"name"`                // 步骤名称
	StepInstanceID   uint64          `json:"step_instance_id"`    // 作业步骤实例ID
	ExecuteCount     int             `json:"execute_count"`       // 步骤重试次数
	CreateTime       int64           `json:"create_time"`         // 作业步骤实例创建时间，Unix时间戳，单位毫秒
	EndTime          int64           `json:"end_time"`            // 作业步骤执行结束时间，Unix时间戳，单位毫秒
	Type             StepType        `json:"type"`                // 步骤类型
	StartTime        int64           `json:"start_time"`          // 作业步骤开始执行时间，Unix时间戳，单位毫秒
	StepIPResultList []*StepIPResult `json:"step_ip_result_list"` // 每个主机的任务执行结果
}

// 主机作业执行状态:1.Agent异常; 5.等待执行; 7.正在执行; 9.执行成功; 11.执行失败; 12.任务下发失败; 403.任务强制终止成功; 404.任务强制终止失败
type StepIPStatus int

const (
	StepIPStatusAgentException   StepIPStatus = 1   // Agent异常
	StepIPStatusWaiting          StepIPStatus = 5   // 等待执行
	StepIPStatusRunning          StepIPStatus = 7   // 正在执行
	StepIPStatusSuccess          StepIPStatus = 9   // 执行成功
	StepIPStatusFailed           StepIPStatus = 11  // 执行失败
	StepIPStatusTaskFailed       StepIPStatus = 12  // 任务下发失败
	StepIPStatusForceStopSuccess StepIPStatus = 403 // 任务强制终止成功
	StepIPStatusForceStopFailed  StepIPStatus = 404 // 任务强制终止失败
)
const (
	StepIPStatusAgentExceptionStr   = "Agent异常"
	StepIPStatusWaitingStr          = "等待执行"
	StepIPStatusRunningStr          = "正在执行"
	StepIPStatusSuccessStr          = "执行成功"
	StepIPStatusFailedStr           = "执行失败"
	StepIPStatusTaskFailedStr       = "任务下发失败"
	StepIPStatusForceStopSuccessStr = "任务强制终止成功"
	StepIPStatusForceStopFailedStr  = "任务强制终止失败"
)

// String get string value of step IP status
func (s StepIPStatus) String() string {
	statusMap := map[StepIPStatus]string{
		StepIPStatusAgentException:   StepIPStatusAgentExceptionStr,
		StepIPStatusWaiting:          StepIPStatusWaitingStr,
		StepIPStatusRunning:          StepIPStatusRunningStr,
		StepIPStatusSuccess:          StepIPStatusSuccessStr,
		StepIPStatusFailed:           StepIPStatusFailedStr,
		StepIPStatusTaskFailed:       StepIPStatusTaskFailedStr,
		StepIPStatusForceStopSuccess: StepIPStatusForceStopSuccessStr,
		StepIPStatusForceStopFailed:  StepIPStatusForceStopFailedStr,
	}
	if str, ok := statusMap[s]; ok {
		return str
	}
	return StepIPStatusAgentExceptionStr
}

// 主机任务状态码，1.Agent异常; 3.上次已成功; 5.等待执行; 7.正在执行; 9.执行成功; 11.任务失败; 12.任务下发失败; 13.任务超时; 15.任务日志错误; 101.脚本执行失败; 102.脚本执行超时; 103.脚本执行被终止; 104.脚本返回码非零; 202.文件传输失败; 203.源文件不存在; 310.Agent异常; 311.用户名不存在; 320.文件获取失败; 321.文件超出限制; 329.文件传输错误; 399.任务执行出错
type StepIPErrorCode int

const (
	StepIPErrorCodeAgentException     StepIPErrorCode = 1   // Agent异常
	StepIPErrorCodeLastSuccess        StepIPErrorCode = 3   // 上次已成功
	StepIPErrorCodeWaiting            StepIPErrorCode = 5   // 等待执行
	StepIPErrorCodeRunning            StepIPErrorCode = 7   // 正在执行
	StepIPErrorCodeSuccess            StepIPErrorCode = 9   // 执行成功
	StepIPErrorCodeFailed             StepIPErrorCode = 11  // 任务失败
	StepIPErrorCodeTaskFailed         StepIPErrorCode = 12  // 任务下发失败
	StepIPErrorCodeTaskTimeout        StepIPErrorCode = 13  // 任务超时
	StepIPErrorCodeTaskLogError       StepIPErrorCode = 15  // 任务日志错误
	StepIPErrorCodeScriptFailed       StepIPErrorCode = 101 // 脚本执行失败
	StepIPErrorCodeScriptTimeout      StepIPErrorCode = 102 // 脚本执行超时
	StepIPErrorCodeScriptTerminated   StepIPErrorCode = 103 // 脚本执行被终止
	StepIPErrorCodeScriptNonZero      StepIPErrorCode = 104 // 脚本返回码非零
	StepIPErrorCodeFileTransferFailed StepIPErrorCode = 202 // 文件传输失败
	StepIPErrorCodeFileNotExist       StepIPErrorCode = 203 // 源文件不存在
	// todo: api 文档目前存在重复的状态，需要确认
	// StepIPErrorCodeAgentException     StepIPErrorCode = 310 // Agent异常
	StepIPErrorCodeUserNameNotExist  StepIPErrorCode = 311 // 用户名不存在
	StepIPErrorCodeFileGetFailed     StepIPErrorCode = 320 // 文件获取失败
	StepIPErrorCodeFileExceedLimit   StepIPErrorCode = 321 // 文件超出限制
	StepIPErrorCodeFileTransferError StepIPErrorCode = 329 // 文件传输错误
	StepIPErrorCodeTaskError         StepIPErrorCode = 399 // 任务执行出错
)

// StepIPResult 主机作业执行结果
type StepIPResult struct {
	BkHostID  uint64          `json:"bk_host_id"`  // 主机ID
	IP        string          `json:"ip"`          // IP地址
	BkCloudID int             `json:"bk_cloud_id"` // 管控区域ID
	Status    StepIPStatus    `json:"status"`      // 作业执行状态
	Tag       string          `json:"tag"`         // 用户通过job_success/job_fail函数模板自定义输出的结果。仅脚本任务存在该参数
	ExitCode  int             `json:"exit_code"`   // 脚本任务exit code
	ErrorCode StepIPErrorCode `json:"error_code"`  // 主机任务状态码
	StartTime int64           `json:"start_time"`  // 开始执行时间，Unix时间戳，单位毫秒
	EndTime   int64           `json:"end_time"`    // 执行结束时间，Unix时间戳，单位毫秒
	TotalTime int64           `json:"total_time"`  // 总耗时（毫秒）
}
