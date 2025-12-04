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

package config

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"

	istep "github.com/Tencent/bk-bcs/bcs-common/common/task/steps/iface"
	"github.com/TencentBlueKing/bk-bscp/internal/components/bcs"
	"github.com/TencentBlueKing/bk-bscp/internal/components/gse"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/repository"
	"github.com/TencentBlueKing/bk-bscp/internal/task/executor/common"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/constant"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	"github.com/TencentBlueKing/bk-bscp/pkg/logs"
	"github.com/TencentBlueKing/bk-bscp/pkg/tools"
)

const (
	// validate push config step name
	ValidatePushConfigStepName istep.StepName = "ValidatePushConfig"
	// download config step name
	DownloadConfigStepName istep.StepName = "DownloadConfig"
	// PushConfigStepName push config step name
	PushConfigStepName istep.StepName = "PushConfig"
	// PushConfigCallbackName push config callback name
	PushConfigCallbackName istep.CallbackName = "PushConfigCallback"
)

// PushConfigExecutor push config executor
type PushConfigExecutor struct {
	*common.Executor
	GseService *gse.Service        // GSE服务，用于文件传输
	Repo       repository.Provider // 仓库服务，用于下载配置文件
}

// NewPushConfigExecutor new push config executor
func NewPushConfigExecutor(dao dao.Set, gseService *gse.Service, repo repository.Provider) *PushConfigExecutor {
	return &PushConfigExecutor{
		Executor: &common.Executor{
			Dao:        dao,
			GseService: gseService,
		},
		GseService: gseService,
		Repo:       repo,
	}
}

// push config payload
type PushConfigPayload struct {
	BizID        uint32
	BatchID      uint32
	OperateType  table.ConfigOperateType
	OperatorUser string
	// ConfigGenerateTaskPayload 配置生成时的渲染结果，包含配置内容、目标路径等信息
	ConfigGenerateTaskPayload *common.TaskPayload
}

// ValidatePushConfig implements istep.Step.
func (e *PushConfigExecutor) ValidatePushConfig(c *istep.Context) error {
	payload := &PushConfigPayload{}
	if err := c.GetPayload(payload); err != nil {
		return err
	}

	return nil
}

// DownloadConfig implements istep.Step.
// DownloadConfig 将配置生成的渲染结果下载到本地文件
func (e *PushConfigExecutor) DownloadConfig(c *istep.Context) error {
	payload := &PushConfigPayload{}
	if err := c.GetPayload(payload); err != nil {
		return err
	}

	// 1. 从 CommonPayload 中获取配置生成的内容
	commonPayload := &common.TaskPayload{}
	if err := c.GetCommonPayload(commonPayload); err != nil {
		return fmt.Errorf("get common payload failed: %w", err)
	}

	if commonPayload.ConfigPayload == nil {
		return fmt.Errorf("config payload not found in common payload")
	}

	// 获取配置内容
	configContent := commonPayload.ConfigPayload.ConfigContent
	signature := commonPayload.ConfigPayload.ConfigContentSignature
	logs.Infof("download config for batch %d, biz_id: %d, config_key: %s",
		payload.BatchID, payload.BizID, commonPayload.ConfigPayload.ConfigInstanceKey)

	// 2. 将配置内容写入本地文件
	// todo: 配置文件路径需要从配置中获取
	sourceDir := path.Join("/tmp/bscp/config/push", strconv.Itoa(int(payload.BizID)))
	if err := os.MkdirAll(sourceDir, os.ModePerm); err != nil {
		return fmt.Errorf("create source directory failed, %s", err.Error())
	}
	// filepath = /tmp/bscp/config/push/{biz_id}/{sha256}
	serverFilePath := path.Join(sourceDir, signature)

	// 将配置内容写入文件
	if err := os.WriteFile(serverFilePath, []byte(configContent), os.ModePerm); err != nil {
		return fmt.Errorf("write config content to file failed, %s", err.Error())
	}
	logs.Infof("write config content to file success, file_path: %s", serverFilePath)

	return nil
}

// getServerInfo 获取当前服务器的AgentID和ContainerID
func getServerInfo() (agentID string, containerID string, err error) {
	ctx := context.Background()
	gseConf := cc.FeedServer().GSE

	if gseConf.NodeAgentID != "" {
		// 如果配置了NodeAgentID，说明feed server部署在主机上
		return gseConf.NodeAgentID, "", nil
	}

	// 如果没有配置NodeAgentID，说明部署在容器中，需要从容器信息中获取
	// 参考 NewScheduler 的实现，使用重试机制获取服务器信息
	retry := tools.NewRetryPolicy(5, [2]uint{3000, 5000})

	var lastErr error
	for {
		if retry.RetryCount() == 5 {
			return "", "", fmt.Errorf("get server agent id and container id failed after 5 retries, last error: %w", lastErr)
		}

		agentID, containerID, lastErr = getServerInfoFromK8s(ctx, gseConf)
		if lastErr != nil {
			logs.Warnf("get server info from k8s failed, retry count: %d, err: %v", retry.RetryCount(), lastErr)
			retry.Sleep()
			continue
		}
		return agentID, containerID, nil
	}
}

// getServerInfoFromK8s 从 K8s 环境中获取服务器的 AgentID 和 ContainerID
func getServerInfoFromK8s(ctx context.Context, gseConf cc.GSE) (agentID string, containerID string, err error) {
	// 检查必需的配置参数
	if gseConf.ClusterID == "" || gseConf.PodID == "" {
		return "", "", fmt.Errorf("server agent_id or (cluster_id and pod_id) is required")
	}

	// 查询 Pod 信息
	pod, err := bcs.QueryPod(ctx, gseConf.ClusterID, gseConf.PodID)
	if err != nil {
		return "", "", fmt.Errorf("query pod failed: %w", err)
	}

	// 从 Pod 的容器状态中查找指定容器的 ContainerID
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == gseConf.ContainerName {
			containerID = tools.SplitContainerID(container.ContainerID)
			break
		}
	}
	if containerID == "" {
		return "", "", fmt.Errorf("server container %s not found in pod %s/%s",
			gseConf.ContainerName, gseConf.ClusterID, gseConf.PodID)
	}

	// 查询 Node 信息，获取 AgentID
	node, err := bcs.QueryNode(ctx, gseConf.ClusterID, pod.Spec.NodeName)
	if err != nil {
		return "", "", fmt.Errorf("query node failed: %w", err)
	}

	agentID = node.Labels[constant.LabelKeyAgentID]
	if agentID == "" {
		return "", "", fmt.Errorf("bk-agent-id not found in server node %s/%s", gseConf.ClusterID, pod.Spec.NodeName)
	}

	logs.Infof("get server info from k8s success, agent_id: %s, container_id: %s", agentID, containerID)
	return agentID, containerID, nil
}

// PushConfig implements istep.Step.
// PushConfig 负责调用 GSE 接口，将本地文件传输到目标机器
func (e *PushConfigExecutor) PushConfig(c *istep.Context) error {
	payload := &PushConfigPayload{}
	if err := c.GetPayload(payload); err != nil {
		return err
	}

	// 校验配置生成结果是否存在
	if payload.ConfigGenerateTaskPayload == nil {
		return fmt.Errorf("config generate task payload not found")
	}
	if payload.ConfigGenerateTaskPayload.ConfigPayload == nil {
		return fmt.Errorf("config payload not found")
	}
	if payload.ConfigGenerateTaskPayload.ProcessPayload == nil {
		return fmt.Errorf("process payload not found")
	}

	configPayload := payload.ConfigGenerateTaskPayload.ConfigPayload
	processPayload := payload.ConfigGenerateTaskPayload.ProcessPayload

	logs.Infof("push config for batch %d, biz_id: %d, config_key: %s",
		payload.BatchID, payload.BizID, configPayload.ConfigInstanceKey)

	kt := kit.New()
	kt.BizID = payload.BizID

	// 1. 构建源文件路径
	sourceDir := path.Join(cc.FeedServer().GSE.CacheDir, strconv.Itoa(int(payload.BizID)))
	fileName := configPayload.ConfigInstanceKey

	// 2. 构建目标 agent 列表
	targetAgents := []gse.TransferFileAgent{
		{
			BkAgentID:     processPayload.AgentID,
			BkContainerID: "", // 进程实例通常在主机上，不在容器中
			User:          configPayload.ConfigFileOwner,
		},
	}

	// 3. 获取源服务器的 AgentID 和 ContainerID
	serverAgentID, serverContainerID, err := getServerInfo()
	if err != nil {
		return fmt.Errorf("get server info failed, %s", err.Error())
	}

	// 4. 获取目标文件路径和文件名
	targetFileDir := configPayload.ConfigFilePath
	targetFileName := configPayload.ConfigFileName

	// 5. 创建 GSE 文件传输任务
	transferFileReq := &gse.TransferFileReq{
		TimeOutSeconds: 600,
		AutoMkdir:      true,
		UploadSpeed:    0,
		DownloadSpeed:  0,
		Tasks: []gse.TransferFileTask{
			{
				Source: gse.TransferFileSource{
					FileName: fileName,
					StoreDir: sourceDir,
					Agent: gse.TransferFileAgent{
						BkAgentID:     serverAgentID,
						BkContainerID: serverContainerID,
						User:          cc.FeedServer().GSE.AgentUser,
					},
				},
				Target: gse.TransferFileTarget{
					FileName: targetFileName,
					StoreDir: targetFileDir,
					Agents:   targetAgents,
				},
			},
		},
	}

	// TODO: 调用 GSE 接口传输文件
	resp, err := e.GseService.AsyncExtensionsTransferFile(kt.Ctx, transferFileReq)
	if err != nil {
		return fmt.Errorf("create gse transfer file task failed, %s", err.Error())
	}

	logs.Infof("create gse transfer file task success, batch_id: %d, gse_task_id: %s, target: %s/%s",
		payload.BatchID, resp.Result.TaskID, targetFileDir, targetFileName)

	// TODO: 保存 GSE 任务 ID，用于后续查询传输结果

	return nil
}

// PushConfigCallback implements istep.Callback.
func (e *PushConfigExecutor) PushConfigCallback(c *istep.Context, cbErr error) error {
	payload := &PushConfigPayload{}
	if err := c.GetPayload(payload); err != nil {
		return fmt.Errorf("[PushConfigCallback]: get payload failed: %w", err)
	}

	// 更新 TaskBatch 状态
	isSuccess := cbErr == nil
	if err := e.Dao.TaskBatch().IncrementCompletedCount(kit.New(), payload.BatchID, isSuccess); err != nil {
		return fmt.Errorf("[PushConfigCallback]: increment completed count failed, batchID: %d, err: %w",
			payload.BatchID, err)
	}

	if isSuccess {
		logs.Infof("[PushConfigCallback]: push config success, batch_id: %d", payload.BatchID)
	} else {
		logs.Errorf("[PushConfigCallback]: push config failed, batch_id: %d, err: %v", payload.BatchID, cbErr)
	}

	return nil
}

// RegisterStepExecutor register step executor
func RegisterPushConfigExecutor(e *PushConfigExecutor) {
	istep.Register(ValidatePushConfigStepName, istep.StepExecutorFunc(e.ValidatePushConfig))
	istep.Register(DownloadConfigStepName, istep.StepExecutorFunc(e.DownloadConfig))
	istep.Register(PushConfigStepName, istep.StepExecutorFunc(e.PushConfig))
	istep.RegisterCallback(PushConfigCallbackName, istep.CallbackExecutorFunc(e.PushConfigCallback))
}
