# 数据模型：进程托管配置定时检查与异常闭环

**需求 ID**：短 ID 135663906 / 长 ID 1020451610135663906
**来源**：`spec.md`（FR-001~FR-015）、`req.md` 技术方案 + 技术澄清补充（attempt-2）、`questions.md` Q-001~Q-011、`research.md` D1~D12
**说明**：本子需求**不新增表/字段**。「托管异常记录」表（`process_managed_exceptions`）由上游 #135663687 提供，本需求只读写。
下列为本需求引入的**进程内运行态结构体**（无持久化），用于解析、比对、决策与配置。

> **attempt-2 关键校正**：`procName` 来源是 **`Process.Spec.FuncName`**（`pkg/dal/table/process.go:110`），**不在 `ProcessInfo` 内**（`ProcessInfo` 字段仅 BkStartParamRegex/WorkPath/PidFile/User/ReloadCmd/RestartCmd/StartCmd/StopCmd/FaceStopCmd/Timeout/StartCheckSecs，无 FuncName）。`BuildProcessOperate` 也是把入参 `FuncName` 写进 `Identity.ProcName`（`internal/processor/gse/gse.go:113`）。

## 1. 上游持久化模型（只读写，不定义）

复用 `pkg/dal/table/process_managed_exception.go`：

| 模型 | 说明 |
|------|------|
| `table.ProcessManagedException` | 三段式：`ID + Attachment + Spec + Revision` |
| `Spec.ErrorType` | `table.ProcessExceptionErrorType`（5 枚举：`PARSING_FAILED`/`AGENT_EXCEPTION`/`ILLEGAL_VALUE_KEY`/`EXPECTATION_MISMATCH`/`OTHER`） |
| `Spec.ErrorMsg` | 差异/原因明细（属性差异时含差异字段名集合；非法项时含非法 valuekey 集合） |
| `Spec.HandlingSuggestion` | 处理建议（对标 gsekit 文案） |
| `Spec.Status` | `table.ProcessExceptionStatus`（`exception`/`recovered`） |
| `Spec.CheckedAt` | 本轮检查时间（写入时传入） |
| `Attachment` | `TenantID/BizID/HostID/ProcessID/ProcessInstanceID`（定位字段，冗余免 join） |

写入约束：
- 异常记录 `Attachment.HostID` 取自所属 `Process.Attachment.HostID`（`process_instances` 表不含 host_id）。
- `Create` 时 ID 由上游 DAO 内部 `idGen.One` 分配；`Revision.Creator/Reviser` 由 kit 上下文/回调填充。

## 2. 运行态结构体（本需求新增，无持久化）

> 置于 `internal/processor/processcheck/`，全部为纯数据结构，便于单包单测。

### 2.1 期望托管项 ExpectedProc（FR-005/FR-006 / Q-008/Q-009/Q-010）

由 `Process` + `ProcessInstance` + 渲染后的 `ProcessInfo` 构造。**比对字段固定为 9 个**（对标 gsekit `proc` 字典子集），其余 GSE 内部字段一律不构造。

| 字段 | 来源 | 说明 |
|------|------|------|
| ValueKey | `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}` | 匹配键（`gse.BuildNamespace`+`gse.BuildProcessName(alias, hostInstSeq)`，用别名 alias）|
| ManagedStatus | `ProcessInstance.Spec.ManagedStatus` | **是否应托管判定基准**（managed/unmanaged/starting/stopping/partly_managed/空）|
| **ProcName** | **`Process.Spec.FuncName`** | **进程二进制名；不在 ProcessInfo 内** |
| SetupPath | 渲染后的 `ProcessInfo.WorkPath` | 工作路径 |
| PidPath | 渲染后的 `ProcessInfo.PidFile` | PID 文件路径 |
| User | `ProcessInfo.User` | 启动账户（直取，不渲染）|
| StartCmd | 渲染后的 `ProcessInfo.StartCmd` | 启动命令 |
| StopCmd | 渲染后的 `ProcessInfo.StopCmd` | 停止命令 |
| RestartCmd | 渲染后的 `ProcessInfo.RestartCmd` | 重启命令 |
| ReloadCmd | 渲染后的 `ProcessInfo.ReloadCmd` | 重载命令 |
| KillCmd | 渲染后的 `ProcessInfo.FaceStopCmd` | 强制停止命令 |
| ProcessInstanceID / ProcessID / HostID / BizID / TenantID / AgentID | `ProcessInstance` / `Process` | 定位与下发目标（不参与属性比对）|

> 渲染复用 `internal/processor/gse.BuildProcessOperate(BuildProcessOperateParams{...})`：其输出 `gse.ProcessOperate.Spec.Identity{ProcName(=FuncName)/SetupPath/PidPath/User}` 与 `Control{StartCmd/StopCmd/RestartCmd/ReloadCmd/KillCmd}` 即上述 9 字段渲染结果，检查器据此填充 ExpectedProc，避免重写 mako 渲染（research D4）。
> **参与属性比对的仅 9 字段**：`procName/setupPath/pidPath/user/startCmd/stopCmd/restartCmd/reloadCmd/killCmd`。

### 2.2 实际托管项 ActualProc（FR-003/FR-004/FR-007 / Q-011）

解析 agent `.proc` Screen JSON（**驼峰命名**）的 `proc` 数组元素，**仅保留 `contact == GSEKIT_BIZ_{bizID}` 的本业务项**。

| 字段 | JSON key（驼峰）| 说明 |
|------|----------|------|
| Contact | `contact` | 本业务过滤键，须 == `GSEKIT_BIZ_{bizID}` |
| ValueKey | `valuekey` | 匹配键（如 `GSEKIT_BIZ_100148:nginx_1`）|
| ProcName | `procName` | 进程名 |
| SetupPath | `setupPath` | 工作路径 |
| PidPath | `pidPath` | PID 路径 |
| User | `user` | 账户 |
| StartCmd / StopCmd / RestartCmd / ReloadCmd / KillCmd | `startCmd`/`stopCmd`/`restartCmd`/`reloadCmd`/`killCmd` | 控制命令 |

> **解析剔除字段**：`versionCmd`/`healthCmd`/`type`/`cpulmt`/`memlmt`/`password`/`userPwd`/`startCheckBeginTime`/`startCheckEndTime`/`opTimeOut`/`operateType`/`timestamp` 不进入比对（即便反序列化保留，比对时只取 9 字段）。否则「期望 ⊆ 实际」恒误判（Q-010）。
> `.proc` 顶层结构对标 gsekit `_parse_ip_logs`：`{"proc":[{...}, ...]}`；用正则从 Screen 抽取首个 `{...}` JSON 对象（DOTALL）后反序列化。
> **单测基准**：`samples/proc-example.json`（3 条 `GSEKIT_BIZ_100148` + 2 条 `nodeman`），验证 contact 过滤、9 字段比对、非法 valuekey 判定。

### 2.3 单实例检查结论 CheckResult（FR-005/FR-008~FR-011）

| 字段 | 说明 |
|------|------|
| ProcessInstanceID / ProcessID / HostID / BizID / TenantID | 定位 |
| Verdict | `exception` / `pass` / `skip`（starting/stopping/partly_managed/不应托管且无 actual）|
| ErrorType `table.ProcessExceptionErrorType` | 异常时填，对标上游枚举（research D5）|
| ErrorMsg / HandlingSuggestion string | 异常明细与建议 |
| CheckedAt time.Time | 本轮检查时间 |

## 3. 比对与分类规则（FR-005/FR-006/FR-007 / 对标 gsekit `_check_process_mismatch`+`_check_single_proc`）

按 host(agentID) 维度：先 host 级错误短路，再过滤本业务项，再算非法集合，最后逐实例按 `ManagedStatus` 判定。

0. **host 级错误（先于一切短路）**：
   - 脚本无结果 / Screen 无法抽取或反序列化 JSON → 该 host 全部相关实例 `PARSING_FAILED`。
   - Screen/错误信息含 "agent not available" 类信号、或 agent 非 normal → 该 host 全部实例 `AGENT_EXCEPTION`。
   - 其余无法归类 → `OTHER`。

1. **本业务过滤**：`actual = [p for p in proc if p.contact == GSEKIT_BIZ_{bizID}]`（非本业务来源如 nodeman 一律丢弃，不参与判定）。

2. **非法 valuekey（host 级）**：
   - `expected_keys` = 该主机**全部** bscp 实例 valuekey（不分 managed/unmanaged）。
   - `illegal_keys = actual_keys - expected_keys`，非空 → 对应实例（或该 host）记 `ILLEGAL_VALUE_KEY`（`error_msg` 含非法 keys 集合）。

3. **逐实例按 `ManagedStatus` 判定**（按 valuekey 取 actual）：

   | `ManagedStatus` | actual 有该 valuekey | actual 无该 valuekey |
   |---|---|---|
   | `managed`（应托管）| 比对 9 字段子集：差异 → `EXPECTATION_MISMATCH`（error_msg 列差异字段名集合）；一致 → **通过** | `EXPECTATION_MISMATCH`（已托管但未获取到信息）|
   | `unmanaged` / `""`（不应托管）| `EXPECTATION_MISMATCH`（未托管却有信息）| **通过**，不记录 |
   | `starting` / `stopping`（过渡态）| **跳过**（无写入）| **跳过**（无写入）|
   | `partly_managed` | **忽略**（实例上不出现）| **忽略** |

4. **9 字段子集比对**：`procName/setupPath/pidPath/user/startCmd/stopCmd/restartCmd/reloadCmd/killCmd` 全部在 actual 对应字段存在且相等即视为一致（对标 gsekit `proc.items() <= actual.items()`）；任一不等记入差异字段集合 → `EXPECTATION_MISMATCH`。

## 4. 配置结构（FR-013 / TQ-001/TQ-002 / research D9）

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
| `Process().ListProcessesWithInstance(kit, bizID)` | `internal/dal/dao/process.go` | 单业务进程（含 Spec.FuncName/Alias/OsType/SourceData）|
| `ProcessInstance().GetByProcessIDs(kit, bizID, processIDs)` | `internal/dal/dao/process_instance.go` | 进程实例（含 Spec.ManagedStatus/HostInstSeq/ModuleInstSeq）|
| `ProcessManagedException().{Create,GetLatestByProcessInstanceID,IsException,UpdateStatus}` | `internal/dal/dao/process_managed_exception.go` | 写异常/恢复（上游 #135663687）|
| `gse.Service.AsyncExtensionsExecuteScript` / `common.Executor.WaitExecuteScriptFinish` | `internal/components/gse` / `internal/task/executor/common` | 下发 cat .proc + 轮询 |
| `internal/processor/gse.BuildProcessOperate` | `internal/processor/gse/gse.go` | 渲染期望 9 字段（ProcName=FuncName）|
| `gse.BuildNamespace` / `gse.BuildProcessName` | `internal/components/gse/type.go` | 构造 valuekey |
| `serviced.Service.IsMaster()` | `internal/serviced/serviced.go` | 主从守卫 |
| `config.GetExecutionUser` / `ScriptStoreDirByFileMode` / `BuildScriptCommand` | `internal/task/executor/config/script_builder.go` | 执行账户/脚本目录/命令（可复用）|

## 6. 关系与不变量

- **应托管基准 = `ProcessInstanceSpec.ManagedStatus`**（bscp 无 is_auto 等价字段）；不为「稳态掉管」新增表字段（已知取舍，spec.md 边界场景）。
- **procName 来源 = `Process.Spec.FuncName`**，其余 8 字段来源 `Process.Spec.SourceData` 反序列化的 `ProcessInfo`。
- 检查粒度 = process_instance；host 级错误扇出到该 agent 下全部相关实例（FR-010）。
- 写入只追加（异常）或翻转最新（恢复），不删除历史；以「最近一次检查结论」为准（FR-011）。
- 不落敏感个人信息：仅写运维类字段（路径/命令/账户名/差异字段名），不含密钥/凭证（FR-015）。
