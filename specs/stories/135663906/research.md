# 技术调研：进程托管配置定时检查与异常闭环

**需求 ID**：短 ID 135663906 / 长 ID 1020451610135663906
**范围**：编排 + 比对（定时巡检 + GSE 实际配置获取 + 子集比对 + 写异常/恢复），复用既有构建块，不新增基建。
所有结论均依据 `context.md` 白名单源码 + `req.md`（含「技术澄清补充（第 2 轮 / attempt-2）」）/ `spec.md` / `questions.md`（Q-001~Q-011）。

> attempt-2 校正：本轮针对「是否应托管基准」「比对字段集合」「.proc 真实格式与 contact 过滤」三处与事实不符或表述过粗的决策做源码核实并重写（见 D3a / D4 / D5）。源码核实结论：`Process.Spec.FuncName` 提供 procName（`ProcessInfo` 内无该字段，已读 `pkg/dal/table/process.go`）；`BuildProcessOperate` 将 `params.FuncName` 写入 `Identity.ProcName`（`internal/processor/gse/gse.go:113`）；应托管基准为 `ProcessInstanceSpec.ManagedStatus`（`pkg/dal/table/process_instance.go:86`，枚举 managed/unmanaged/starting/stopping/partly_managed）。

## D1 — GSE 实际配置获取：复用脚本执行构建块，不复用 istep 流水线（FR-003 / Q-001）

- **决策**：复用「GSE 异步脚本执行 + 结果轮询 + Screen 解析」三件套构建块，即
  `gse.Service.AsyncExtensionsExecuteScript`（`internal/components/gse/script.go`）下发 → `common.Executor.WaitExecuteScriptFinish`（`internal/task/executor/common/common.go`）轮询 →
  取 `ExecuteScriptResult.Result[0].Screen` 解析。**不**复用 `internal/task/executor/config/config_check.go` 的 `CheckConfigExecutor` istep step/callback 流水线。
- **理由**：`config_check.go` 的价值在 istep 任务编排（step/callback/payload/通知）；本巡检无需任务流水线，仅需「下发 cat .proc → 拿 Screen」这一段。直连更轻量，符合 AGENTS.md「不引入不必要抽象」（Q-001、spec.md 范围外条目）。
- **复用方式**：在检查器内构造一个仅填充 `GseService/GseConf/TaskConf` 的 `common.Executor`（与 `NewCheckConfigExecutor` 同样的构造），调用其 `WaitExecuteScriptFinish(ctx, taskID, agentID)`。该方法内部用 `GetExecuteScriptResult` 轮询、`TaskConf.ScriptExecution.PollInterval/PollTimeout` 控制超时，已被配置检查链路验证。
- **备选**：直接裸调 `GetExecuteScriptResult` 自写轮询 → 否决（重复造轮子，放弃既有超时/容错语义）。

## D2 — cat .proc 脚本与执行账户（FR-004 / TQ-001 / Q-001）

- **决策**：`.proc` 读取命令做成配置项，缺省对标 gsekit `SCRIPT_CONTENT`：
  - linux：`cat /usr/local/gse2_bkte/agent/etc/.proc`
  - windows：`type c:\gse2_bkte\agent\etc\.proc`
  执行账户对标 gsekit `ACCOUNT_ALIAS`：linux=`root`，windows=`Administrator`（复用 `config.GetExecutionUser(fileMode, "")` 的缺省返回值，无需新写）。
- **理由**：gsekit `check_process.py` 的 `get_script_content()` 即从 `GlobalSettings.CHECK_PROC_SCRIPT` 取、缺省 `SCRIPT_CONTENT`；路径随 agent 版本/部署可能变化（TR-001），配置化可改。`.proc` 是 agent 维护的托管进程清单文件，需具备权限账户读取。
- **下发结构**：沿用 `config_check.go` 的 `gse.ExecuteScriptReq`：单 `Agent{BkAgentID, User}` + 单 `Script{ScriptName, ScriptStoreDir, ScriptContent}` + 单 `AtomicTask{Command, TimeoutSeconds: TaskConf.ScriptExecution.TimeoutSec}`。脚本以 `cat .proc` 作为 ScriptContent，命令用 `BuildScriptCommand`。`ScriptStoreDir` 复用 `GseConf.ScriptStoreDir`/`WindowsScriptStoreDir`（`ScriptStoreDirByFileMode`）。
- **OS 维度**：按 `Process.Spec.OsType`（"linux"/"win"，对标 `table.FileMode`）选 linux/windows 命令与账户，逐 OS/agent 下发（gsekit 按 os_type 分组）。

## D3 — 实际托管项与期望项的匹配键、本业务 contact 过滤（FR-007 / Q-002 / Q-009 / Q-011）

- **决策**：`namespace`/`contact` = `GSEKIT_BIZ_{bizID}`（`gse.BuildNamespace`，`NamespacePrefix="GSEKIT_BIZ_"`）；期望 valuekey = `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`（用进程**别名 alias**，对应 `gse.BuildProcessName(alias, hostInstSeq)` 拼出的 `{alias}_{hostInstSeq}` 与 `BuildNamespace` 组合）。
- **本业务过滤（attempt-2 强调）**：`.proc` 为**驼峰命名 JSON**（`{"proc":[{...}]}`），一个主机的 `.proc` 含**多来源**托管项（样例中 `contact=GSEKIT_BIZ_100148` 为 bscp/gsekit 托管，`contact=nodeman` 为节点管理插件）。解析后**必须先按 `proc.contact == GSEKIT_BIZ_{bizID}` 过滤**仅保留本业务托管项，再以 valuekey 与期望项匹配/比对。否则 `nodeman` 等其它来源项会被误判为 `ILLEGAL_VALUE_KEY`（等价 gsekit `_parse_ip_logs` 的 `if proc.get("contact") == self.contact`）。
- **理由**：bscp 注册 GSE 进程用 `internal/processor/gse.BuildProcessOperate` 构造 `Meta.Namespace=BuildNamespace(bizID)`、`Meta.Name=BuildProcessName(alias, hostInstSeq)`；gsekit `check_process.py` 的 `contact=f"GSEKIT_BIZ_{biz}"`、`valuekey=f"{contact}:{procName}_{local_inst_id}"`。两侧前缀/分隔符一致（Q-002）。

## D3a — 「是否应托管」判定基准：ProcessInstanceSpec.ManagedStatus（FR-005 / Q-008，attempt-2 校正）

- **决策**：bscp **无** gsekit `is_auto` 那种独立「期望托管」字段；最接近且唯一可用的是 `ProcessInstanceSpec.ManagedStatus`（`pkg/dal/table/process_instance.go:86`，由 `syncCmdbGse` 周期同步回本地的「bscp 侧最新已知托管态」）。检查以它为「是否应托管」基准，把 gsekit 的 `is_auto`（True/False）替换为 `ManagedStatus`（managed/unmanaged），其余比对算法完全对标 gsekit `check_process.py`：

  | `ManagedStatus` | 含义 | 判定 |
  |---|---|---|
  | `managed` | 应托管 | 本业务 `.proc` 必须有该 valuekey 且 9 字段一致；缺该 valuekey 或属性不符 → `EXPECTATION_MISMATCH` |
  | `unmanaged` / `""`（空，未同步过）| 不应托管 | 不应有该 valuekey；有 → `EXPECTATION_MISMATCH`（未托管却有信息）；无 → 正常**不记录** |
  | `starting` / `stopping` | 操作过渡态 | 本轮**跳过**该实例，避免操作窗口误报 |
  | `partly_managed` | 仅进程维度、实例上不出现 | **忽略** |

- **已知取舍**：若某实例被非法掉管后 `syncCmdbGse` 抢先把 `ManagedStatus` 刷为 `unmanaged`，该「稳态掉管」当轮不告警（由 `syncCmdbGse` 在控制台反映）；本期接受该取舍，**不**为此新增表字段。检查核心价值（**配置属性漂移** + **非法托管项**）不受影响（spec.md 边界场景、Q-008）。
- **理由**：源码核实 `ProcessManagedStatus` 仅 5 个枚举值，无 is_auto 等价；以 ManagedStatus 为基准是改动最小、不引入新表字段的可行路径（AGENTS.md「不引入不必要配置/字段」）。

## D4 — 参与比对的 9 字段集合与渲染（FR-006 / Q-006 / Q-010，attempt-2 定稿）

- **决策**：期望项**只构造并比对以下 9 个字段**（bscp 经 `BuildProcessOperate` 实际下发的字段），按「期望项 ⊆ 实际项」子集比对（对标 gsekit `proc.items() <= actual.items()`）：

  | `.proc` 字段（驼峰）| bscp 来源 | 渲染/取值 |
  |---|---|---|
  | `procName` | **`Process.Spec.FuncName`**（注意：**不在 `ProcessInfo` 内**）| 直取，无需 mako 渲染 |
  | `setupPath` | `ProcessInfo.WorkPath` | 渲染后 |
  | `pidPath` | `ProcessInfo.PidFile` | 渲染后 |
  | `user` | `ProcessInfo.User` | 直取（启动账户）|
  | `startCmd` | `ProcessInfo.StartCmd` | 渲染后 |
  | `stopCmd` | `ProcessInfo.StopCmd` | 渲染后 |
  | `restartCmd` | `ProcessInfo.RestartCmd` | 渲染后 |
  | `reloadCmd` | `ProcessInfo.ReloadCmd` | 渲染后 |
  | `killCmd` | `ProcessInfo.FaceStopCmd` | 渲染后 |

- **必须剔除（attempt-2 关键）**：`versionCmd`/`healthCmd`（bscp 不下发，样例中为空字符串）以及 `type`/`cpulmt`/`memlmt`/`password`/`userPwd`/`startCheck*`/`opTimeOut`/`operateType`/`timestamp`（GSE agent 内部字段，bscp 不下发不关心）。若把这些纳入期望项，「期望 ⊆ 实际」会因 bscp 侧无对应期望值而**恒误判**（详见 Q-010）。
- **复用渲染（不重写 mako）**：直接复用 `internal/processor/gse.BuildProcessOperate(BuildProcessOperateParams{BizID, Alias, FuncName, HostInstSeq, ModuleInstSeq, SetName, ModuleName, ProcessInfo})`：其输出 `gse.ProcessOperate.Spec.Identity/Control` 即上述渲染结果——`Identity.ProcName=FuncName`、`Identity.SetupPath=WorkPath(渲染)`、`Identity.PidPath=PidFile(渲染)`、`Identity.User=User`、`Control.{Start/Stop/Restart/Reload/Kill}Cmd`。检查器据此映射 9 字段，避免重写一套渲染（源码核实 `gse.go:104-131`）。
- **比对语义（子集，对标 gsekit `_check_single_proc`）**：9 字段全部在 actual 对应字段中存在且相等即视为一致；任一字段不等（或 actual 缺该字段值）→ `EXPECTATION_MISMATCH`，`error_msg` 写出**差异字段名集合**。
- **范围说明**：gsekit 比对项里的 `versionCmd/healthCmd` 在 bscp 侧无下发来源，按 spec.md FR-006 显式收窄到 9 字段，属对标 gsekit 的有意收窄（避免恒误判），记入决策。

## D5 — 异常类别 → 上游枚举映射（FR-008 / Q-003）

- **决策**：检查侧异常类别一律落到上游 `table.ProcessExceptionErrorType` 5 枚举（`pkg/dal/table/process_managed_exception.go`，已就绪：`PARSING_FAILED`/`AGENT_EXCEPTION`/`ILLEGAL_VALUE_KEY`/`EXPECTATION_MISMATCH`/`OTHER`）：

  | 检查侧情形 | 上游枚举 | 对标 gsekit |
  |-----------|---------|------------|
  | 应托管（managed）但本业务 `.proc` 无该 valuekey | `EXPECTATION_MISMATCH` | `_check_single_proc` 已托管无 actual |
  | 不应托管（unmanaged/空）但本业务 `.proc` 有该 valuekey | `EXPECTATION_MISMATCH` | `_check_single_proc` 未托管有 actual |
  | 9 字段属性不一致 | `EXPECTATION_MISMATCH` | `proc.items() <= actual.items()` 失败 |
  | 本业务 actual valuekey 不在 host 级期望集合 | `ILLEGAL_VALUE_KEY` | `_check_process_mismatch` illegal_keys |
  | 脚本无结果 / Screen 抽取或 JSON 反序列化失败 | `PARSING_FAILED` | `_parse_ip_logs` except 分支 / no_log_hosts |
  | 脚本日志含 "agent not available" 类信号 / agent 非 normal | `AGENT_EXCEPTION` | `_parse_ip_logs` agent 分支 |
  | 其余无法归类 | `OTHER` | `ErrorType.OTHER` |

- **理由**：上游表枚举已对标 gsekit `ErrorType`，本需求只读写不重定义；差异细节落 `error_msg`，处置建议落 `handling_suggestion`（对标 gsekit `handling_suggestion` 文案）。

## D6 — valuekey 集合与非法项判定（FR-007 / Q-009，attempt-2 定稿）

- **决策**（host(agentID) 维度）：
  - 期望集合 `expected_keys` = 该主机上**全部** bscp 进程实例的 valuekey（`GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`），**不区分 managed/unmanaged**，只要 bscp 认识该实例就纳入。
  - 实际集合 `actual_keys` = 本业务过滤后（contact==GSEKIT_BIZ_{bizID}）`.proc` 项的 valuekey 集合。
  - `illegal_keys = actual_keys - expected_keys` → 记 `ILLEGAL_VALUE_KEY`（host 级，`error_msg` 含非法 keys 集合）。
  - 对 `expected_keys` 内每个实例，再按 D3a 的 `ManagedStatus` 分支逐一判定（缺失/属性不符 → `EXPECTATION_MISMATCH`）。
- **理由**：gsekit `_check_process_mismatch` 即 `illegal_keys = set(actual) - set(expected)`；expected 用全部本业务实例 valuekey，避免「未托管实例的 actual 项」被误判为非法。

## D7 — 写异常/恢复闭环判定与写入路径（FR-009/FR-010/FR-011 / Q-007）

- **决策**：以 process_instance 为单位每轮得出「异常/通过/跳过」结论：
  - **异常** → `dao.ProcessManagedException.Create` 追加一条 `status=exception` 记录（含 `error_type/error_msg/handling_suggestion/checked_at` + 定位 `tenant_id/biz_id/host_id/process_id/process_instance_id`）。
  - **通过** → `IsException(biz, instID)`==true（最新记录为 exception）时，先 `GetLatestByProcessInstanceID` 取最新记录 id，再 `UpdateStatus(biz, latest.ID, recovered)` 完成闭环；最新记录非 exception（无记录或已 recovered）时不动作。
  - **跳过**（starting/stopping/partly_managed/不应托管且无 actual）→ 无写入、无状态更新。
- **host_id 来源**：`process_instances` 表 Attachment 不含 host_id，取自所属 `Process.Attachment.HostID` 冗余写入（与上游 #135663687 data-model 一致）。
- **理由**：上游 DAO（`Create/GetLatestByProcessInstanceID/IsException/UpdateStatus`）契约已就绪且稳定（#135663687）；「以最近一次检查结论为准」由「取最新记录判定 + 追加写/翻转」实现。`UpdateStatus` 仅传 `recovered`，复用其刷新 `reviser/updated_at`。

## D8 — host 级错误扇出与错误隔离（FR-010/FR-012 / AC-002/AC-T02）

- **决策**：
  - host 级错误（脚本无结果、Screen JSON 解析失败、agent 异常）→ 扇出到该主机（agentID）下**全部相关进程实例**各写一条对应 `error_type`（`PARSING_FAILED`/`AGENT_EXCEPTION`），对标 gsekit `_add_error(host_ids,...)` / no_log_hosts 扇出。
  - 单业务/单主机/单进程任一环节失败：仅记录该范围异常并 `logs.Errorf`，`continue` 处理下一个，不阻断其余（对标 `sync_cmdb.go` 跨租户 continue、`syncBizHostByTenant` 跨 biz continue、`buildBizOperateItems` 跨进程 continue）。
  - 写库 / `UpdateStatus` 失败：仅记日志，下一轮重试（不在本轮重试、不阻断其余实例）。
- **理由**：检查粒度以 process_instance 落库（上游主键定位到 process_instance_id），host 级错误必须落到该 host 全部实例才能被操作侧按实例判定。

## D9 — 定时任务接入与配置（FR-001/FR-013 / Q-004 / TQ-002）

- **决策**：在 `CrontabConfig`（`pkg/cc/types.go`）新增子配置 `CheckProcessManagedConfig`（yaml key `checkProcessManaged`），字段：
  - `Enabled bool`（缺省 false，向后兼容）
  - `Interval string`（缺省 `20m`，对标 `SyncCmdbGseConfig`）
  - `QpsLimit float64`（缺省 `80.0`，量级与 `SyncCmdbGse`/`WatchBizHostRelation` 一致）
  - `LinuxProcScript string`（缺省 `cat /usr/local/gse2_bkte/agent/etc/.proc`）
  - `WindowsProcScript string`（缺省 `type c:\gse2_bkte\agent\etc\.proc`）
  补 `validate()`（interval 可 `time.ParseDuration`、qpsLimit>=0）与 `trySetDefault()`，并在 `CrontabConfig.validate()/trySetDefault()` 中串接；`data_service.yaml` 增 `checkProcessManaged` 样例（默认 `enabled: false`）。
- **任务体**：新增 `cmd/data-service/service/crontab/check_managed_process.go`，沿用 `sync_cmdb.go` 样板：`Run()`：`time.NewTicker(interval)` + `shutdown.AddNotifier()` + `select{notifier.Signal / ticker.C}`，`ticker.C` 分支先 `if !state.IsMaster() { continue }`（AC-004/AC-T01/SC-004）。在 `startCronTasks()` 中按 `crontabConfig.CheckProcessManaged.Enabled` 守卫启动（与既有任务一致），依赖 `ds.daoSet / ds.sd / ds.gseSvc`。
- **QPS 粒度（TQ-002 定稿）**：单任务持有一个 `rate.NewLimiter(rate.Limit(qpsLimit), 1)`，巡检全程（跨 biz、跨 agent 下发）共享，每次 GSE 下发前 `rateLimiter.Wait(ctx)`——即**全局限流**粒度（对标 `sync_cmdb.go`/`sync_biz_host.go` 单 limiter）。不做每业务独立 limiter（避免不必要复杂度）。

## D10 — 按业务跨租户遍历与并发（FR-002/FR-014 / Q-005 / AC-P01）

- **决策**：`App().ListBizTenantMap(kit)` 取全部 `biz→tenant` 映射；逐业务以 `kit.NewWithTenant(tenant)`（或克隆 kit 设 TenantID + `InternalRpcCtx()`，对标 `sync_biz_host.go`）处理；单业务内 `Process().ListProcessesWithInstance(kit, bizID)` + `ProcessInstance().GetByProcessIDs(kit, bizID, processIDs)`（对标 `SyncSingleBiz`）。
- **并发/分批**：按 agentID 分组下发 cat .proc（一个 agent 一次脚本即可拿到该 host 全部托管项）；以 host(agentID) 为并发单元、信号量限并发上限（对标 `sync_gse.go` 并发控制），叠加全局 `rateLimiter.Wait` 限速（D9）。业务之间串行（每轮串行，配合 IsMaster 守卫避免重入，TR-003）。
- **理由**：单 agent 的 `.proc` 已含该主机全部托管进程，无需逐进程下发；按 agent 聚合显著降低 GSE 调用次数（SC-007）。

## D11 — 测试基建与可测性（测试策略 / AGENTS.md 测试优先）

- **单测基准数据（attempt-2 定稿）**：以 `samples/proc-example.json` 作为 `.proc` Screen 解析/比对单测基准。该样例含 3 条 `contact=GSEKIT_BIZ_100148`（nginx_1/2/3，valuekey `GSEKIT_BIZ_100148:nginx_1` 等）+ 2 条 `contact=nodeman`（bkmonitorbeat/bksecbeat），正好用于验证：
  - contact 过滤（nodeman 项不参与判定、不误报为 ILLEGAL_VALUE_KEY，SC-006）；
  - 9 字段子集比对（驼峰 key 反序列化 + 剔除 versionCmd/healthCmd/内部字段后比对一致）；
  - 非法 valuekey 判定（构造期望集合缺某 valuekey 时 actual 多出 → ILLEGAL_VALUE_KEY）。
- **纯函数单测（优先，单包可验证）**：把以下逻辑实现为不依赖 DB/GSE 的纯函数，置于核心检查 internal 包：
  - `.proc` Screen → JSON `proc` 列表解析（含空 Screen、非 JSON、含 "agent not available"、按 contact 过滤本业务）→ 覆盖 FR-004、AC-T02/SC-005、AGENT_EXCEPTION。
  - 期望项 valuekey 构造（`GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`，FR-007/Q-009）。
  - host 级 illegal_keys = actual - expected（FR-007，AC-T03/SC-006）。
  - 按 `ManagedStatus` 分支 + 9 字段子集比对 → `error_type` 分类（应托管无 actual / 不应托管有 actual / 属性差异 / 一致 / starting|stopping 跳过 / partly_managed 忽略）→ 覆盖 FR-005/FR-006、AC-001/AC-T03。
  - 单实例检查结论 → 写入/恢复动作决策（异常 Create / 通过且 IsException→UpdateStatus / 否则 no-op）→ 覆盖 FR-009~FR-011、AC-001/AC-T04。
- **可测性 seam**：为「在某 agent 上执行 cat .proc 并返回 Screen 文本」定义一个最小接口（实参由 `common.Executor`+`gse.Service` 实现），使编排逻辑可用 fake 注入做集成测试（mock 各类 Screen/错误码，验证写异常/恢复与「单主机失败不阻断其余」，覆盖 AC-001/AC-002/AC-003）。DAO 侧用上游已存在的 `dao.ProcessManagedException` 接口，测试以 fake 实现注入。
- **不可仅单测部分**：跨租户遍历 + 真实 GSE 下发 + 真实 DB 写入的端到端路径依赖带 DB/GSE 环境，本期以集成 mock + 代码评审保障（对标 #135663687 plan-report 取舍）。
- **不引入新依赖**：不引入 sqlite/sqlmock（与运行时方言不一致）；纯函数 + 接口 fake 即可覆盖核心判定。该取舍在 plan-report testability 记录。

## D12 — 代码模块归属（AGENTS.md：目录归属明确、不引入不必要抽象）

- **决策**：
  - 定时任务入口：`cmd/data-service/service/crontab/check_managed_process.go`（Run/ticker/IsMaster/限流/跨业务编排）。
  - 核心检查逻辑：新增内部包 `internal/processor/processcheck/`（`parse.go` Screen 解析+contact 过滤、`expected.go` 期望项构造、`compare.go` 比对+分类、`record.go` 写异常/恢复决策、`executor.go` ScriptRunner seam、`checker.go` 单业务编排）。该包复用 `internal/components/gse`、`internal/task/executor/common`、`internal/processor/gse`（BuildProcessOperate 渲染）、`internal/dal/dao`。
  - 配置：`pkg/cc/types.go` + `pkg/cc/service.go`（若默认值/校验在 service.go）+ `cmd/data-service/etc/data_service.yaml`。
- **理由**：定时任务样板归 crontab 包（与既有一致）；比对/解析为独立业务域逻辑，单独 internal 包便于纯函数单测且不污染既有 processor/gse；复用而非重写既有 dao/gse/processor 能力（符合白名单约束与 AGENTS.md）。新增一个内部包属必要的领域边界，非冗余抽象。
