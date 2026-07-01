# 数据模型：进程托管配置定时检查与异常闭环

**需求 ID**：短 ID 135663906 / 长 ID 1020451610135663906
**来源**：`spec.md`（FR-001~FR-013）、`req.md` 技术方案、`questions.md` Q-001~Q-007
**说明**：本子需求**不新增表/字段**。「托管异常记录」表（`process_managed_exceptions`）由上游 #135663687 提供，本需求只读写。
下列为本需求引入的**进程内运行态结构体**（无持久化），用于解析、比对、决策与配置。

## 1. 上游持久化模型（只读写，不定义）

复用 `pkg/dal/table/process_managed_exception.go`：

| 模型 | 说明 |
|------|------|
| `table.ProcessManagedException` | 三段式：`ID + Attachment + Spec + Revision` |
| `Spec.ErrorType` | `table.ProcessExceptionErrorType`（5 枚举：`PARSING_FAILED`/`AGENT_EXCEPTION`/`ILLEGAL_VALUE_KEY`/`EXPECTATION_MISMATCH`/`OTHER`） |
| `Spec.ErrorMsg` | 差异/原因明细（含差异字段集合） |
| `Spec.HandlingSuggestion` | 处理建议（对标 gsekit 文案） |
| `Spec.Status` | `table.ProcessExceptionStatus`（`exception`/`recovered`） |
| `Spec.CheckedAt` | 本轮检查时间（写入时传入） |
| `Attachment` | `TenantID/BizID/HostID/ProcessID/ProcessInstanceID`（定位字段，冗余免 join） |

写入约束：
- 异常记录 `Attachment.HostID` 取自所属 `Process.Attachment.HostID`（`process_instances` 表不含 host_id，见上游 D1）。
- `Create` 时 ID 由上游 DAO 内部 `idGen.One` 分配；`Revision.Creator/Reviser` 由 kit 上下文/回调填充。

## 2. 运行态结构体（本需求新增，无持久化）

> 置于 `internal/processor/processcheck/`，全部为纯数据结构，便于单包单测。

### 2.1 期望托管项 ExpectedProc（FR-006 / Q-002 / Q-006）

由 `Process` + `ProcessInstance` + 渲染后的 `ProcessInfo` 构造。比对字段对标 gsekit `proc` 字典子集。

| 字段 | 来源 | 说明 |
|------|------|------|
| ValueKey | `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}` | 匹配键（`gse.BuildNamespace`+`BuildProcessName`） |
| ProcName | `ProcessInfo.FuncName` | 进程二进制名 |
| SetupPath | 渲染后的 `ProcessInfo.WorkPath` | 工作路径 |
| PidPath | 渲染后的 `ProcessInfo.PidFile` | PID 文件路径 |
| StartCmd / StopCmd / RestartCmd / ReloadCmd / KillCmd | 渲染后的对应 `ProcessInfo` 字段（`FaceStopCmd→KillCmd`） | 控制命令 |
| User | `ProcessInfo.User` | 启动账户 |
| ProcessInstanceID / ProcessID / HostID / BizID / AgentID | `ProcessInstance` / `Process` | 定位与下发目标 |

> 渲染复用 `internal/processor/gse.BuildProcessOperate`：其输出 `gse.ProcessOperate.Spec.Identity/Control` 即上述渲染结果，检查器据此填充 ExpectedProc，避免重写 mako 渲染（research D4）。

### 2.2 实际托管项 ActualProc（FR-003/FR-004 / Q-001/Q-002）

解析 agent `.proc` Screen JSON 的 `proc` 数组元素（仅保留 `contact == GSEKIT_BIZ_{bizID}` 的项）。

| 字段 | JSON key | 说明 |
|------|----------|------|
| Contact | `contact` | 本业务过滤键 `GSEKIT_BIZ_{bizID}` |
| ValueKey | `valuekey` | 匹配键 |
| ProcName | `procName` | 进程名 |
| SetupPath | `setupPath` | 工作路径 |
| PidPath | `pidPath` | PID 路径 |
| StartCmd / StopCmd / RestartCmd / ReloadCmd / KillCmd | `startCmd`/`stopCmd`/`restartCmd`/`reloadCmd`/`killCmd` | 控制命令 |
| User | `user`（如有） | 账户 |

> `.proc` 顶层结构对标 gsekit `_parse_ip_logs`：`{"proc":[{...}, ...]}`；用正则 `\{.*\}`（DOTALL）从 Screen 抽取首个 JSON 对象后反序列化。

### 2.3 单实例检查结论 CheckResult（FR-005/FR-007~FR-009）

| 字段 | 说明 |
|------|------|
| ProcessInstanceID / ProcessID / HostID / BizID / TenantID | 定位 |
| IsException bool | 本轮是否异常 |
| ErrorType `table.ProcessExceptionErrorType` | 异常时填，对标上游枚举（research D5） |
| ErrorMsg / HandlingSuggestion string | 异常明细与建议 |
| CheckedAt time.Time | 本轮检查时间 |

## 3. 比对与分类规则（FR-005 / Q-003 / 对标 gsekit `_check_process_mismatch`+`_check_single_proc`）

按 host(agentID) 维度，先过滤本业务项，再逐实例判定：

1. **非法 valuekey**：`actualKeys - expectedKeys` 非空 → 该主机相关实例记 `ILLEGAL_VALUE_KEY`（`error_msg` 含非法 keys 集合）。对标 gsekit illegal_keys 整 host 处理。
2. **逐期望项**（按 valuekey 取 actual）：
   - 期望托管 + actual 缺失 → `EXPECTATION_MISMATCH`（"已托管但未获取到信息"）。
   - 期望未托管 + actual 存在 → `EXPECTATION_MISMATCH`（"未托管但获取到信息"）。
   - 期望托管 + actual 存在：比对 §2.1 字段子集是否为 actual 对应字段子集；存在差异 → `EXPECTATION_MISMATCH`（`error_msg` 列出差异字段名集合）。无差异 → 通过。
3. **host 级错误**（先于 1/2 短路）：
   - 脚本无结果 / Screen 无法抽取或反序列化 JSON → 该 host 全部实例 `PARSING_FAILED`。
   - Screen/错误信息含 "agent not available" 类信号、或 agent 非 normal → `AGENT_EXCEPTION`。
   - 其余无法归类 → `OTHER`。

> "期望是否托管"：bscp 注册进程默认按常驻托管（`AutoTypePersistent`）下发，期望集合内项即视为应托管；若后续区分需以 `ProcessInstance.Spec.ManagedStatus` 细化，本期按"期望集合 = 应托管集合"对标 gsekit `is_auto`。

## 4. 配置结构（FR-011 / TQ-001/TQ-002 / research D8）

`pkg/cc/types.go` 新增（挂到 `CrontabConfig`，yaml key `checkProcessManaged`）：

```go
// CheckProcessManagedConfig 进程托管配置定时检查任务配置
type CheckProcessManagedConfig struct {
    Enabled           bool    `yaml:"enabled"`
    Interval          string  `yaml:"interval"`          // 缺省 20m
    QpsLimit          float64 `yaml:"qpsLimit"`          // 缺省 80.0，全局限流
    LinuxProcScript   string  `yaml:"linuxProcScript"`   // 缺省 cat /usr/local/gse2_bkte/agent/etc/.proc
    WindowsProcScript string  `yaml:"windowsProcScript"` // 缺省 type c:\gse2_bkte\agent\etc\.proc
}
```

- `trySetDefault()`：Interval 空→`20m`，QpsLimit 0→`80.0`，两个脚本字段空→对标 gsekit 缺省。
- `validate()`：Interval 非空时可 `time.ParseDuration`；QpsLimit>=0。
- `CrontabConfig.trySetDefault()/validate()` 串接调用；`data_service.yaml` 增 `checkProcessManaged` 样例（默认 `enabled: false`，纯新增向后兼容）。

## 5. 复用的既有接口（不新增 DAO/表）

| 能力 | 来源 | 用途 |
|------|------|------|
| `App().ListBizTenantMap(kit)` | `internal/dal/dao/app.go` | 跨租户 biz→tenant 映射 |
| `Process().ListProcessesWithInstance(kit, bizID)` | `internal/dal/dao/process.go` | 单业务进程 |
| `ProcessInstance().GetByProcessIDs(kit, bizID, processIDs)` | `internal/dal/dao/process_instance.go` | 进程实例 |
| `ProcessManagedException().{Create,GetLatestByProcessInstanceID,IsException,UpdateStatus}` | `internal/dal/dao/process_managed_exception.go` | 写异常/恢复（上游 #135663687） |
| `gse.Service.AsyncExtensionsExecuteScript` / `common.Executor.WaitExecuteScriptFinish` | `internal/components/gse` / `internal/task/executor/common` | 下发 cat .proc + 轮询 |
| `internal/processor/gse.BuildProcessOperate` | `internal/processor/gse/gse.go` | 渲染期望托管字段 |
| `serviced.Service.IsMaster()` | `internal/serviced/serviced.go` | 主从守卫 |
| `config.GetExecutionUser` / `ScriptStoreDirByFileMode` / `BuildScriptCommand` | `internal/task/executor/config/script_builder.go` | 执行账户/脚本目录/命令（可复用） |

## 6. 关系与不变量

- 检查粒度 = process_instance；host 级错误扇出到该 agent 下全部相关实例（FR-008）。
- 写入只追加（异常）或翻转最新（恢复），不删除历史；以"最近一次检查结论"为准（FR-009）。
- 不落敏感个人信息：仅写运维类字段（路径/命令/账户名/差异字段名），不含密钥/凭证（FR-013）。
