# 技术调研：进程托管配置定时检查与异常闭环

**需求 ID**：短 ID 135663906 / 长 ID 1020451610135663906
**范围**：编排 + 比对（定时巡检 + GSE 实际配置获取 + 逐项比对 + 写异常/恢复），复用既有构建块，不新增基建。
所有结论均依据 `context.md` 白名单源码 + `req.md` / `spec.md` / `questions.md`。

## D1 — GSE 实际配置获取：复用脚本执行构建块，不复用 istep 流水线（FR-003 / Q-001）

- **决策**：复用「GSE 异步脚本执行 + 结果轮询 + Screen 解析」三件套构建块，即
  `gse.Service.AsyncExtensionsExecuteScript`（`internal/components/gse/script.go`）下发 → `common.Executor.WaitExecuteScriptFinish`（`internal/task/executor/common/common.go`）轮询 →
  取 `ExecuteScriptResult.Result[0].Screen` 解析。**不**复用 `internal/task/executor/config/config_check.go` 的 `CheckConfigExecutor` istep step/callback 流水线。
- **理由**：`config_check.go` 的价值在 istep 任务编排（step/callback/payload/通知）；本巡检无需任务流水线，仅需"下发 cat .proc → 拿 Screen"这一段。直连更轻量，符合 AGENTS.md「不引入不必要抽象」（Q-001、spec.md 范围外条目）。
- **复用方式**：在检查器内构造一个仅填充 `GseService/GseConf/TaskConf` 的 `common.Executor`（与 `NewCheckConfigExecutor` 同样的构造），调用其 `WaitExecuteScriptFinish(ctx, taskID, agentID)`。该方法内部用 `GetExecuteScriptResult` 轮询、`TaskConf.ScriptExecution.PollInterval/PollTimeout` 控制超时，已被配置检查链路验证。
- **备选**：直接裸调 `GetExecuteScriptResult` 自写轮询 → 否决（重复造轮子，放弃既有超时/容错语义）。

## D2 — cat .proc 脚本与执行账户（FR-004 / TQ-001 / Q-001）

- **决策**：`.proc` 读取命令做成配置项，缺省对标 gsekit `SCRIPT_CONTENT`：
  - linux：`cat /usr/local/gse2_bkte/agent/etc/.proc`
  - windows：`type c:\gse2_bkte\agent\etc\.proc`
  执行账户对标 gsekit `ACCOUNT_ALIAS`：linux=`root`，windows=`Administrator`（复用 `config.GetExecutionUser(fileMode, "")` 的缺省返回值，无需新写）。
- **理由**：gsekit `check_process.py` 的 `get_script_content()` 即从 `GlobalSettings.CHECK_PROC_SCRIPT` 取、缺省 `SCRIPT_CONTENT`；路径随 agent 版本/部署可能变化（TR-001），配置化可改。`.proc` 是 agent 维护的托管进程清单文件，需具备权限账户读取。
- **下发结构**：沿用 `config_check.go` 的 `gse.ExecuteScriptReq`：单 `Agent{BkAgentID, User}` + 单 `Script{ScriptName, ScriptStoreDir, ScriptContent}` + 单 `AtomicTask{Command, TimeoutSeconds: TaskConf.ScriptExecution.TimeoutSec}`。脚本直接以 `cat .proc` 作为 ScriptContent，命令用 `BuildScriptCommand`。`ScriptStoreDir` 复用 `GseConf.ScriptStoreDir`/`WindowsScriptStoreDir`（`ScriptStoreDirByFileMode`）。
- **OS 维度**：按 `Process.Spec.OsType`（"linux"/"win"，对标 `table.FileMode`）选 linux/windows 命令与账户，逐 OS/agent 下发（gsekit 按 os_type 分组）。

## D3 — 实际托管项与期望项的匹配键（FR-005/FR-006 / Q-002）

- **决策**：`namespace`/`contact` = `GSEKIT_BIZ_{bizID}`（`gse.BuildNamespace`，`NamespacePrefix="GSEKIT_BIZ_"`）；期望 valuekey = `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`。
  解析 `.proc` 时按 `proc.contact == GSEKIT_BIZ_{bizID}` 过滤本业务托管项，再以 valuekey 与期望项逐一匹配。
- **理由**：bscp 注册 GSE 进程用 `internal/processor/gse.BuildProcessOperate` 构造 `Meta.Namespace=BuildNamespace(bizID)`、`Meta.Name=BuildProcessName(alias, hostInstSeq)`；gsekit `check_process.py` 的 `contact=f"GSEKIT_BIZ_{biz}"`、`valuekey=f"{contact}:{procName}_{local_inst_id}"`。两侧完全一致（Q-002）。本业务过滤等价 gsekit `_parse_ip_logs` 的 `if proc.get("contact") == self.contact`。

## D4 — 期望比对字段与渲染（FR-006 / Q-006）

- **决策**：期望配置取自 `Process.Spec.SourceData` 反序列化为 `table.ProcessInfo`（不调 CMDB）。参与"属性不一致"比对的字段固定为 8+1：
  `WorkPath`(→setupPath)、`PidFile`(→pidPath)、`StartCmd`、`StopCmd`、`RestartCmd`、`ReloadCmd`、`FaceStopCmd`(→killCmd)、`User`，外加进程名 `FuncName`(→procName)。
  其中 `WorkPath/PidFile/各 Cmd` 需按实例 `host_inst_seq/module_inst_seq` 上下文渲染后再比对。
- **复用渲染**：直接复用 `internal/processor/gse.BuildProcessOperate(BuildProcessOperateParams{...})` 渲染期望字段——它已将 `ProcessInfo.WorkPath→Identity.SetupPath`、`PidFile→PidPath`、各 Cmd→`Control.*Cmd`、`User→Identity.User`、`FuncName→Identity.ProcName` 全部渲染好（与注册下发口径一致）。检查器据此构造期望项，避免重写一套 mako 渲染。
- **比对语义（子集，对标 gsekit `proc.items() <= actual.items()`）**：期望项的上述字段集合是实际项对应字段的子集即视为一致；存在差异字段则记 `EXPECTATION_MISMATCH`，`error_msg` 写出差异字段集合（对标 gsekit `_check_single_proc` 的 `diff`）。
- **范围说明**：gsekit 还比对 `versionCmd/healthCmd/contact`，本期按 spec.md FR-006 收敛为上述子集（`BuildProcessOperate` 本就不渲染 version/health），属对标 gsekit 的有意收窄，记入决策。

## D5 — 异常类别 → 上游枚举映射（FR-005 / Q-003）

- **决策**：检查侧异常类别一律落到上游 `table.ProcessExceptionErrorType` 5 枚举（`pkg/dal/table/process_managed_exception.go`，已就绪）：
  | 检查侧情形 | 上游枚举 | 对标 gsekit |
  |-----------|---------|------------|
  | 已托管但 GSE 无托管信息 | `EXPECTATION_MISMATCH` | `_check_single_proc` 已托管无 actual |
  | 未托管但 GSE 有托管信息 | `EXPECTATION_MISMATCH` | `_check_single_proc` 未托管有 actual |
  | 配置属性不一致 | `EXPECTATION_MISMATCH` | `proc.items() <= actual.items()` 失败 |
  | 实际 valuekey 不在期望集合 | `ILLEGAL_VALUE_KEY` | `_check_process_mismatch` illegal_keys |
  | 脚本无日志/JSON 解析失败 | `PARSING_FAILED` | `_parse_ip_logs` except 分支 / no_log_hosts |
  | 脚本日志含 "agent not available" / agent 非 normal | `AGENT_EXCEPTION` | `_parse_ip_logs` agent 分支 |
  | 其余无法归类 | `OTHER` | `ErrorType.OTHER` |
- **理由**：上游表枚举已对标 gsekit `ErrorType`，本需求只读写不重定义；差异细节落 `error_msg`，处置建议落 `handling_suggestion`（对标 gsekit `handling_suggestion` 文案）。

## D6 — 写异常/恢复闭环判定与写入路径（FR-007/FR-008/FR-009 / Q-007）

- **决策**：以 process_instance 为单位每轮得出"异常/通过"结论：
  - **异常** → `dao.ProcessManagedException.Create` 追加一条 `status=exception` 记录（含 `error_type/error_msg/handling_suggestion/checked_at` + 定位 `tenant_id/biz_id/host_id/process_id/process_instance_id`）。
  - **通过** → `IsException(biz, instID)`==true（最新记录为 exception）时，先 `GetLatestByProcessInstanceID` 取最新记录 id，再 `UpdateStatus(biz, latest.ID, recovered)` 完成闭环；最新记录非 exception（无记录或已 recovered）时不动作。
- **host_id 来源**：`process_instances` 表 Attachment 不含 host_id，取自所属 `Process.Attachment.HostID` 冗余写入（与上游 data-model D1/§2.2 一致）。
- **理由**：上游 DAO（`Create/GetLatestByProcessInstanceID/IsException/UpdateStatus`）契约已就绪且稳定（#135663687）；"以最近一次检查结论为准"由"取最新记录判定 + 追加写/翻转"实现。`UpdateStatus` 仅传 `recovered`，复用其刷新 `reviser/updated_at`。

## D7 — host 级错误扇出与错误隔离（FR-008/FR-010 / AC-002/AC-T02）

- **决策**：
  - host 级错误（脚本无结果、Screen JSON 解析失败、agent 异常）→ 扇出到该主机（agentID）下**全部相关进程实例**各写一条对应 `error_type`（`PARSING_FAILED`/`AGENT_EXCEPTION`），对标 gsekit `_add_error(host_ids,...)` / no_log_hosts 扇出。
  - 单业务/单主机/单进程任一环节失败：仅记录该范围异常并 `logs.Errorf`，`continue` 处理下一个，不阻断其余（对标 `sync_cmdb.go` 跨租户 continue、`syncBizHostByTenant` 跨 biz continue、`buildBizOperateItems` 跨进程 continue）。
  - 写库/`UpdateStatus` 失败：仅记日志，下一轮重试（不在本轮重试、不阻断其余实例）。
- **理由**：检查粒度以 process_instance 落库（上游主键定位到 process_instance_id），host 级错误必须落到该 host 全部实例才能被操作侧按实例判定（决策记录"检查粒度"）。

## D8 — 定时任务接入与配置（FR-001/FR-011 / Q-004 / TQ-002）

- **决策**：在 `CrontabConfig`（`pkg/cc/types.go`）新增子配置 `CheckProcessManagedConfig`（yaml key `checkProcessManaged`），字段：
  - `Enabled bool`
  - `Interval string`（缺省 `20m`，对标 `SyncCmdbGseConfig`）
  - `QpsLimit float64`（缺省 `80.0`，量级与 `SyncCmdbGse`/`WatchBizHostRelation` 一致）
  - `LinuxProcScript string`（缺省 `cat /usr/local/gse2_bkte/agent/etc/.proc`）
  - `WindowsProcScript string`（缺省 `type c:\gse2_bkte\agent\etc\.proc`）
  补 `validate()`（interval 可解析、qpsLimit>=0）与 `trySetDefault()`，并在 `CrontabConfig.validate()/trySetDefault()` 中串接；`data_service.yaml` 增样例。
- **任务体**：新增 `cmd/data-service/service/crontab/check_managed_process.go`，沿用 `sync_cmdb.go` 样板：`NewCheckManagedProcess(set, sd, gseSvc, qpsLimit, interval, procScript...)` → `Run()`：`time.NewTicker(interval)` + `shutdown.AddNotifier()` + `select{notifier.Signal / ticker.C}`，`ticker.C` 分支先 `if !state.IsMaster() { continue }`（AC-004/AC-T01/SC-004）。在 `startCronTasks()` 中按 `crontabConfig.CheckProcessManaged.Enabled` 守卫启动（与既有任务一致），依赖 `ds.daoSet / ds.sd / ds.gseSvc`。
- **理由**：完全对齐既有 crontab 任务接入方式（Q-004），纯新增配置项默认可关，未开启不影响既有行为（向后兼容）。
- **QPS 粒度（TQ-002 定稿）**：单任务持有一个 `rate.NewLimiter(rate.Limit(qpsLimit), 1)`，巡检全程（跨 biz、跨 agent 下发）共享，每次 GSE 下发前 `rateLimiter.Wait(ctx)`——即**全局限流**粒度（对标 `sync_cmdb.go`/`sync_biz_host.go` 单 limiter）。不做每业务独立 limiter（避免不必要复杂度）。

## D9 — 按业务跨租户遍历与并发（FR-002/FR-012 / Q-005 / AC-P01）

- **决策**：`App().ListBizTenantMap(kit)` 取全部 `biz→tenant` 映射；逐业务以 `kit.NewWithTenant(tenant)`（或克隆 kit 设 TenantID + `InternalRpcCtx()`，对标 `sync_biz_host.go`）处理；单业务内 `Process().ListProcessesWithInstance(bizID)` + `ProcessInstance().GetByProcessIDs(bizID, processIDs)`（对标 `SyncSingleBiz`）。
- **并发/分批**：按 agentID 分组下发 cat .proc（一个 agent 一次脚本即可拿到该 host 全部托管项）；以 host(agentID) 为并发单元、信号量限并发上限（对标 `sync_gse.go` `bizSyncConcurrency`），叠加全局 `rateLimiter.Wait` 限速（D8）。业务之间串行（每轮串行，配合 IsMaster 守卫避免重入，TR-003）。
- **理由**：单 agent 的 `.proc` 已含该主机全部托管进程，无需逐进程下发；按 agent 聚合显著降低 GSE 调用次数（SC-006）。

## D10 — 测试基建与可测性（测试策略 / AGENTS.md 测试优先）

- **纯函数单测（优先，单包可验证）**：把以下逻辑实现为不依赖 DB/GSE 的纯函数，置于核心检查 internal 包：
  - `.proc` Screen → JSON `proc` 列表解析（含空 Screen、非 JSON、含 "agent not available"、按 contact 过滤本业务）→ 覆盖 FR-004、AC-T02/SC-005、AGENT_EXCEPTION。
  - 期望项 valuekey 构造（FR-006/Q-002）。
  - 逐项比对 + `error_type` 分类（已托管无信息/未托管有信息/属性差异/非法 valuekey/一致五类）→ 覆盖 FR-005、AC-T03。
  - 单实例检查结论 → 写入/恢复动作决策（异常 Create / 通过且 IsException→UpdateStatus / 否则 no-op）→ 覆盖 FR-007~FR-009、AC-001/AC-T04。
- **可测性 seam**：为"在某 agent 上执行 cat .proc 并返回 Screen 文本"定义一个最小接口（实参由 `common.Executor`+`gse.Service` 实现），使编排逻辑可用 fake 注入做集成测试（mock 各类 Screen/错误码，验证写异常/恢复与"单主机失败不阻断其余"，覆盖 AC-001/AC-002/AC-003）。DAO 侧用上游已存在的 `dao.ProcessManagedException` 接口，测试以 fake 实现注入。
- **不可仅单测部分**：跨租户遍历 + 真实 GSE 下发 + 真实 DB 写入的端到端路径依赖带 DB/GSE 环境，本期以集成 mock + 代码评审保障（对标 #135663687 plan-report D9 取舍）。
- **不引入新依赖**：不引入 sqlite/sqlmock（与运行时方言不一致）；纯函数 + 接口 fake 即可覆盖核心判定。该取舍在 plan-report testability 记录。

## D11 — 代码模块归属（AGENTS.md：目录归属明确、不引入不必要抽象）

- **决策**：
  - 定时任务入口：`cmd/data-service/service/crontab/check_managed_process.go`（Run/ticker/IsMaster/限流/跨业务编排）。
  - 核心检查逻辑：新增内部包 `internal/processor/processcheck/`（`checker.go` 编排单业务检查、`parse.go` Screen 解析、`compare.go` 比对+分类、`record.go` 写异常/恢复决策）。该包复用 `internal/components/gse`、`internal/task/executor/common`、`internal/processor/gse`（BuildProcessOperate 渲染）、`internal/dal/dao`。
  - 配置：`pkg/cc/types.go` + `pkg/cc/service.go`（若默认值/校验在 service.go）+ `cmd/data-service/etc/data_service.yaml`。
- **理由**：定时任务样板归 crontab 包（与既有一致）；比对/解析为独立业务域逻辑，单独 internal 包便于纯函数单测且不污染既有 processor/gse；复用而非重写既有 dao/gse/processor 能力（符合白名单约束与 AGENTS.md）。新增一个内部包属必要的领域边界，非冗余抽象。
