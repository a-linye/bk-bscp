# Clarification Questions — Story 135663906

> 阶段：clarify / attempt 1 / round 1
> 约定：`resolved_by_doc` 表示已由白名单文档自答并写入 req.md 技术澄清章节；`open` 表示需外部代问；`answered` 表示已获外部答复。

## Q-001 复用配置检查机制的具体方式（GSE 异步脚本读取 .proc）
- 状态：resolved_by_doc
- 依据：`internal/task/executor/config/config_check.go`、`internal/task/executor/common/common.go`、`internal/components/gse/script.go`、`bk-process-config-manager/apps/gsekit/process/handlers/check_process.py`
- 结论：复用的是「GSE 异步脚本执行 + 结果轮询 + Screen 解析」这一组构建块，而非整套 istep 任务流水线。
  即：构造 `cat .proc` 脚本（对标 gsekit `SCRIPT_CONTENT`：linux `cat /usr/local/gse2_bkte/agent/etc/.proc`、windows `type c:\gse2_bkte\agent\etc\.proc`）→
  `GseService.AsyncExtensionsExecuteScript` 下发 →
  `WaitExecuteScriptFinish`（内部 `GetExecuteScriptResult`）轮询 →
  取 `result.Result[0].Screen` 解析出 JSON 的 `proc` 列表。
  不引入 istep `CheckConfigExecutor` 的 step/callback 流水线（避免不必要抽象，符合 AGENTS.md）。

## Q-002 GSE 实际托管项与 bscp 期望项的匹配键（namespace/contact/valuekey）
- 状态：resolved_by_doc
- 依据：`internal/components/gse/type.go`（`NamespacePrefix="GSEKIT_BIZ_"`、`BuildNamespace`、`BuildProcessName`、`BuildResultKey`）、`internal/processor/gse/gse.go`（`BuildProcessOperate` 用 `Namespace=GSEKIT_BIZ_{bizID}`、`Name={alias}_{hostInstSeq}`）、gsekit `check_process.py`（`contact=GSEKIT_BIZ_{biz}`、`valuekey={contact}:{procName}_{localInstId}`）
- 结论：bscp 注册 GSE 进程用的 namespace 与 gsekit contact 完全一致（`GSEKIT_BIZ_{bizID}`）。
  期望 valuekey = `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`。
  解析 .proc 时按 `contact == GSEKIT_BIZ_{bizID}` 过滤本业务托管项，再以 valuekey 与期望项逐一匹配。

## Q-003 异常类别枚举与上游 process_managed_exception 表枚举的对应关系
- 状态：resolved_by_doc
- 依据：`pkg/dal/table/process_managed_exception.go`（`ProcessExceptionErrorType`）、gsekit `check_process.py`（`ErrorType`）、req.md F-001 步骤 4
- 结论：检查侧异常类别 → 上游枚举映射：
  - 已托管但 GSE 无托管信息 → `EXPECTATION_MISMATCH`
  - 未托管但 GSE 有托管信息 → `EXPECTATION_MISMATCH`
  - 配置属性不一致 → `EXPECTATION_MISMATCH`
  - 非法托管项（actual valuekey 不在期望集合）→ `ILLEGAL_VALUE_KEY`
  - 获取/解析失败（无脚本日志、JSON 解析失败）→ `PARSING_FAILED`
  - agent 异常（脚本日志含 "agent not available" 类信号 / agent 非 normal）→ `AGENT_EXCEPTION`
  - 其余无法归类 → `OTHER`

## Q-004 定时周期与配置项接入方式
- 状态：resolved_by_doc
- 依据：`pkg/cc/types.go`（`CrontabConfig`/`SyncCmdbGseConfig` 的 Enabled/Interval/QpsLimit + `trySetDefault`/`validate`）、`pkg/cc/service.go`、`cmd/data-service/app/app.go`（`startCronTasks` 按 `crontabConfig.X.Enabled` 守卫启动）、`cmd/data-service/service/crontab/sync_cmdb.go`
- 结论：在 `CrontabConfig` 新增一个子配置（结构同 `SyncCmdbGseConfig`：`Enabled`/`Interval`/`QpsLimit`），默认 `Interval=20m`、`QpsLimit` 取与现有任务一致量级（建议 80）。
  在 `startCronTasks` 中按 `Enabled` 守卫启动新巡检任务，任务体沿用 sync_cmdb 样板：`time.NewTicker(interval)` + `shutdown.AddNotifier()` + `IsMaster()` 守卫。

## Q-005 按业务遍历的数据源与跨租户遍历
- 状态：resolved_by_doc
- 依据：`internal/dal/dao/app.go`（`ListBizTenantMap` 跨租户 `biz→tenant` 映射，内部 `WithSkipTenantFilter`）、`internal/processor/gse/sync_gse.go`（`SyncSingleBiz`：`ListProcessesWithInstance` + `GetByProcessIDs` + 分批并发 + 限流样板）、`internal/components/bkuser/bkuser.go`
- 结论：用 `App().ListBizTenantMap(kit)` 得到全部 `biz_id→tenant_id`，逐业务以 `kit.NewWithTenant(tenant)` 处理；
  单业务内取数沿用 `SyncSingleBiz`：`Process().ListProcessesWithInstance(bizID)` + `ProcessInstance().GetByProcessIDs(bizID, processIDs)`，按 agentID/批次分组下发、对 GSE 调用限流。
  单业务/单主机失败仅记录、不阻断其余业务（对应 AC-002）。

## Q-006 "配置属性不一致" 的比对粒度（参与比对的 ProcessInfo 字段）
- 状态：resolved_by_doc
- 依据：`pkg/dal/table/process.go`（`ProcessInfo`）、gsekit `check_process.py`（`_check_single_proc` 用 `proc.items() <= actual_proc.items()` 子集比对）、req.md F-001 步骤 2
- 结论：参与比对的期望字段取 `ProcessInfo` 中与托管相关的：`WorkPath`(setupPath)、`PidFile`(pidPath)、`StartCmd`、`StopCmd`、`RestartCmd`、`ReloadCmd`、`FaceStopCmd`(killCmd)、`User`(启动用户)，外加进程名 `FuncName`(procName)。
  比对语义对标 gsekit：期望项为实际项的子集即视为一致，存在差异字段则记 `EXPECTATION_MISMATCH` 并在 `error_msg` 写出差异字段集合。
  渲染期望配置取自 bscp DB（`Process.Spec.SourceData` 反序列化为 `ProcessInfo`），本期不调 CMDB。

## Q-007 异常自动恢复（F-002）的判定与写入路径
- 状态：resolved_by_doc
- 依据：`internal/dal/dao/process_managed_exception.go`（`GetLatestByProcessInstanceID`/`IsException`/`UpdateStatus`/`Create`）、`specs/stories/135663687/spec.md` FR-008/FR-009、req.md F-002
- 结论：每轮按进程实例得出检查结论：
  - 异常 → `Create` 追加一条 `status=exception` 记录（含 error_type/error_msg/handling_suggestion/checked_at）。
  - 通过 → 若 `IsException`==true（最新记录为 exception），对最新记录 `UpdateStatus(recovered)` 完成闭环；否则无需动作。
  以"最近一次检查结论"为准；UpdateStatus 失败仅记日志，下一轮重试。

---

> 以下 Q-008~Q-011 为 attempt-2（confirm 评审深挖）补充，状态 answered（源码核实 + 用户决策 + 真实 .proc 样例）。

## Q-008 「是否应托管」的判定基准（bscp 有无 is_auto 等价字段）
- 状态：answered
- 依据：`pkg/dal/table/process_instance.go`（`ProcessManagedStatus`：starting/stopping/managed/unmanaged/partly_managed）、`internal/processor/gse/sync_gse.go:499` `parseGSEProcResult`（由 GSE 返回 IsAuto 推导 managed/unmanaged）、`docs/reqs/进程状态同步修复.md`/`进程状态同步优化.md`（syncCmdbGse 周期同步 managed_status）
- 结论：bscp **无** gsekit `is_auto` 等价的独立「期望托管」字段；以 `ProcessInstanceSpec.ManagedStatus` 为基准（它是 syncCmdbGse 周期同步的「bscp 侧最新已知托管态」）。映射：
  - `managed` → 应托管（必须有 .proc 项且配置一致）
  - `unmanaged` / `""` → 不应托管（不应有 .proc 项；有则「未托管却有信息」）
  - `starting` / `stopping` → 操作过渡态，本轮跳过
  - `partly_managed` → 仅进程维度，实例上不出现，忽略
- 取舍：稳态掉管若被 syncCmdbGse 抢先刷为 unmanaged 则当轮不告警（由 syncCmdbGse 在控制台反映）；不为此新增表字段。检查核心价值（配置属性漂移 + 非法托管项）不受影响。

## Q-009 期望项集合与 valuekey 构造、非法项判定
- 状态：answered
- 依据：`internal/processor/gse/gse.go`（`BuildProcessName(alias, hostInstSeq)`、`BuildNamespace`）、gsekit `check_process.py`（expected_keys/illegal_keys 逻辑）
- 结论：期望 valuekey = `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`（用别名 alias）；host 级 expected_keys = 该主机全部 bscp 实例 valuekey（不分 managed/unmanaged）；`illegal = actual - expected` → `ILLEGAL_VALUE_KEY`；expected 内逐实例按 Q-008 的 ManagedStatus 分支判定。

## Q-010 参与比对的字段集合（对照真实 .proc 定稿）
- 状态：answered
- 依据：真实 `.proc` 样例（用户提供）、`internal/processor/gse/gse.go` `BuildProcessOperate`（Identity+Control 实际下发字段）、`internal/components/gse/type.go`
- 结论：只比对 bscp 实际下发的 9 字段：`procName`(来源 `Process.Spec.FuncName`，非 ProcessInfo) / `setupPath`(WorkPath) / `pidPath`(PidFile) / `user`(User) / `startCmd` / `stopCmd` / `restartCmd` / `reloadCmd` / `killCmd`(FaceStopCmd)。按「期望项 ⊆ 实际项」子集比对。
  - **剔除** `versionCmd`/`healthCmd`（bscp 不下发，样例为空）及 `type/cpulmt/memlmt/password/userPwd/startCheck*/opTimeOut/operateType/timestamp`（GSE 内部字段）。否则子集比对恒误判。

## Q-011 .proc 真实格式与本业务过滤
- 状态：answered
- 依据：真实 `.proc` 样例（用户提供）
- 结论：`.proc` 为驼峰命名 JSON（`{"proc":[{...}]}`），与 gsekit 解析一致。一个主机 `.proc` 含多来源托管项（`contact=GSEKIT_BIZ_{biz}` 为本业务，`contact=nodeman` 等为其它来源）。解析后**必须按 `contact==GSEKIT_BIZ_{bizID}` 过滤**仅留本业务项，再匹配/比对，否则 nodeman 等会被误判为非法 valuekey。
