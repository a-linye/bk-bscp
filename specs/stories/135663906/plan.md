# 实现计划：进程托管配置定时检查与异常闭环

**需求 ID**：短 ID 135663906 / 长 ID 1020451610135663906
**分支**：不新建分支（在当前工作分支实现）
**输入**：`spec.md`（FR-001~FR-015、SC-001~007、AC-001~004/AC-P01/AC-T01~T04）、`research.md`（D1~D12）、`data-model.md`
**模式**：测试驱动开发（TDD）—— 解析/比对/分类/决策等纯逻辑先写测试再实现
**轮次**：attempt-2（confirm 评审深挖后回退重做）

## 摘要

为父需求"GSE 托管信息检查"提供**核心检查引擎**：在 data-service 新增一个定时巡检任务（仅 master 执行），按业务跨租户遍历进程实例，复用「GSE 异步脚本执行 + 结果轮询 + Screen 解析」构建块读取 agent `.proc`，**以 `ProcessInstanceSpec.ManagedStatus` 为「是否应托管」基准**、按 `contact==GSEKIT_BIZ_{bizID}` 过滤本业务托管项，对裁剪后的 **9 字段**（`procName` 来源 `Process.Spec.FuncName`）做「期望 ⊆ 实际」子集比对，将异常写入上游「托管异常记录」并在恢复时闭环。**复用而非重写**既有 crontab 样板、GSE 脚本执行、`BuildProcessOperate` 渲染、上游异常记录 DAO；**不**复用 `config_check.go` 的 istep 流水线，不新增表/字段，纯新增配置项向后兼容。

## attempt-2 校正要点（相对 attempt-1）

| # | 校正点 | 落点 |
|---|--------|------|
| 1 | 应托管基准改为 `ProcessInstanceSpec.ManagedStatus`（不引入 is_auto 等价字段）| research D3a / data-model §2.1+§3 / 步骤 4 |
| 2 | 比对字段裁剪为 **9 字段**，显式剔除 versionCmd/healthCmd 及 GSE 内部字段 | research D4 / data-model §2.1+§2.2 / 步骤 3+5 |
| 3 | `procName` 来源订正为 `Process.Spec.FuncName`（**非 ProcessInfo**）| research D4 / data-model §2.1+§6 / 步骤 3 |
| 4 | `.proc` 为驼峰 JSON，解析后必须 `contact==GSEKIT_BIZ_{bizID}` 过滤 | research D3 / data-model §2.2+§3 / 步骤 2 |
| 5 | valuekey=`GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`（别名 alias）；host 级 illegal=actual-expected | research D3/D6 / data-model §3 / 步骤 4 |
| 6 | 子集比对对标 gsekit `proc.items() <= actual.items()`；差异字段集合入 error_msg | research D4 / data-model §3 / 步骤 5 |
| 7 | 单测以 `samples/proc-example.json` 为基准（含 GSEKIT_BIZ_ 与 nodeman 混合）| research D11 / 步骤 2+5 |

## 技术上下文（Technical Context）

| 项 | 取值 |
|----|------|
| 语言 / 运行时 | Go（仓库现有版本，见 `go.mod`）|
| 运行位置 | data-service 进程内 crontab 任务，仅 `IsMaster()` 执行 |
| GSE 调用 | `internal/components/gse`（`AsyncExtensionsExecuteScript`/`GetExecuteScriptResult`）+ `internal/task/executor/common.Executor.WaitExecuteScriptFinish` 轮询 |
| 期望渲染 | `internal/processor/gse.BuildProcessOperate`（复用，渲染 ProcName=FuncName + ProcessInfo 8 字段）|
| 应托管基准 | `ProcessInstance.Spec.ManagedStatus`（bscp 无 is_auto 等价字段）|
| 数据源 DAO | `App().ListBizTenantMap`、`Process().ListProcessesWithInstance`、`ProcessInstance().GetByProcessIDs` |
| 写异常/恢复 | 上游 `dao.ProcessManagedException`（Create/GetLatest/IsException/UpdateStatus，#135663687 已就绪）|
| 限流/并发 | `golang.org/x/time/rate` 全局 limiter（缺省 80 QPS）+ 信号量并发上限（对标 `sync_gse.go`）|
| 配置 | `pkg/cc/types.go` `CrontabConfig` 新增 `CheckProcessManagedConfig` + `pkg/cc` 默认/校验 + `data_service.yaml` 样例 |
| 测试 | 纯函数单包单测（解析/比对/分类/决策，基准 `samples/proc-example.json`）+ 接口 fake 集成（GSE/DAO）；不引入 sqlite/sqlmock |
| 新增外部依赖 | 无 |
| 待澄清项 | 无（Q-001~Q-011 全部 resolved_by_doc / answered；TQ-001/TQ-002 在本计划定稿，见 research D2/D9）|

## 约束 / 合规自检基线（替代 Constitution Check）

> 本仓库无 `.specify/memory/constitution.md`，约束以 `AGENTS.md` 为准。

- [x] **语言**：协议字段/枚举/配置键保持英文（`GSEKIT_BIZ_`、`error_type`、9 个驼峰 key 等沿用既有）；注释中文，仅解释业务约束（ManagedStatus 基准、contact 过滤、9 字段裁剪原因、host_id 来源、恢复判定、限流粒度）。
- [x] **Go 规范**：符合 `.golangci.yml`；改动后 `gofmt`；复用既有命名风格（`NewXxx`/ticker 样板/`Validate()`）。
- [x] **不引入不必要抽象**：不复用 istep step/callback 流水线（research D1）；不为「稳态掉管」新增表字段（research D3a）；唯一新增的可测性 seam 是"执行 cat .proc 取 Screen"最小接口（research D11），属必要测试边界。
- [x] **复用优先**：crontab 样板、GSE 脚本执行、`BuildProcessOperate` 渲染（含 ProcName=FuncName）、上游 DAO 全部复用，不重写。
- [x] **纯新增向后兼容**：新增 crontab 任务默认 `Enabled=false`，未开启不影响既有行为；不改既有表/DAO/任务。
- [x] **数据保护**：仅写运维类字段（路径/命令/账户名/差异字段名），不落敏感个人信息（FR-015）。
- [x] **生成文件**：本需求不触达 `internal/dal/gen/`（不新增表），无 `make gen`。

## 项目结构 / 文件级改动（按依赖顺序）

```
pkg/cc/types.go                                   [改动] 新增 CheckProcessManagedConfig + 挂到 CrontabConfig；validate()/trySetDefault() 串接
pkg/cc/service.go                                 [改动] 若默认/校验在此（与既有 crontab 一致处）补串接
cmd/data-service/etc/data_service.yaml            [改动] crontab 下新增 checkProcessManaged 样例（enabled:false 缺省）
internal/processor/processcheck/                  [新增] 核心检查逻辑包
  ├─ parse.go                                     [新增] .proc 驼峰 JSON Screen → []ActualProc 解析 + 本业务 contact 过滤 + agent 异常/解析失败识别
  ├─ parse_test.go                                [新增] 解析单测（基准 samples/proc-example.json：contact 过滤/驼峰反序列化/空/非JSON/agent not available）
  ├─ expected.go                                  [新增] 由 Process/Instance/ProcessInfo 构造 ExpectedProc（ProcName=Process.Spec.FuncName；其余 8 字段复用 BuildProcessOperate 渲染）
  ├─ compare.go                                   [新增] host 级 illegal_keys + 按 ManagedStatus 分支 + 9 字段子集比对 + error_type 分类
  ├─ compare_test.go                              [新增] 比对/分类单测（基准样例：managed/unmanaged/starting|stopping/partly_managed + 属性差异 + 非法 valuekey）
  ├─ record.go                                    [新增] 单实例结论 → Create / UpdateStatus / no-op 决策（依赖上游 DAO 接口）
  ├─ record_test.go                               [新增] 写入/恢复决策单测（fake DAO 验证 Create/IsException+UpdateStatus/no-op）
  ├─ executor.go                                  [新增] ScriptRunner 接口 + 基于 common.Executor/gse.Service 的实现（cat .proc → Screen）
  └─ checker.go                                   [新增] 单业务编排：取数→按 agent 分组下发→解析→比对→落库；错误隔离
cmd/data-service/service/crontab/check_managed_process.go  [新增] 定时任务入口：ticker+shutdown+IsMaster+跨业务遍历+限流
cmd/data-service/app/app.go                       [改动] startCronTasks() 中按 CheckProcessManaged.Enabled 守卫启动（注入 daoSet/sd/gseSvc）
```

## Phase 0 — 调研（见 research.md）

已完成，无 NEEDS CLARIFICATION 未决项。关键决策：复用脚本执行构建块不复用 istep(D1)、cat .proc 脚本/账户配置化(D2)、匹配键+contact 过滤(D3)、**ManagedStatus 应托管基准(D3a)**、**9 字段裁剪 + procName=FuncName + BuildProcessOperate 渲染(D4)**、异常枚举映射(D5)、valuekey 集合与非法项判定(D6)、写异常/恢复路径(D7)、host 扇出与错误隔离(D8)、配置接入与 QPS 全局限流(D9)、跨租户遍历与并发(D10)、可测性 seam + samples 基准(D11)、模块归属(D12)。

## Phase 1 — 设计（见 data-model.md）

不新增表/字段；引入运行态结构体 ExpectedProc（含 ManagedStatus + 9 比对字段，ProcName 来源 FuncName）/ ActualProc（驼峰解析 + contact 过滤）/ CheckResult（含 skip 态）与配置结构 CheckProcessManagedConfig。复用上游 `process_managed_exceptions` 表与 DAO。比对/分类规则见 data-model §3。本需求为后台巡检逻辑，不对外暴露新 API/契约（无 contracts/、无 quickstart）。

## Phase 2 — TDD 实现顺序

> 每步对应 tasks 阶段一条/多条任务。纯逻辑步骤遵循"先写测试→红→实现→绿"。

1. **配置项（FR-013 / TQ-001/TQ-002）**
   - `pkg/cc/types.go`：新增 `CheckProcessManagedConfig`（Enabled/Interval/QpsLimit/LinuxProcScript/WindowsProcScript），挂到 `CrontabConfig`；写 `trySetDefault()`（20m / 80.0 / gsekit 缺省脚本）与 `validate()`（interval 可解析、qpsLimit>=0）；在 `CrontabConfig.trySetDefault()/validate()` 串接。
   - `cmd/data-service/etc/data_service.yaml`：crontab 下增 `checkProcessManaged` 样例（`enabled: false`）。
   - 验证：`gofmt` + `go build ./pkg/cc/...`（如有 cc 单测则 `go test ./pkg/cc/...`）。

2. **.proc 解析 + contact 过滤（FR-003/FR-004/FR-007，AC-T02/SC-005/SC-006）— TDD**
   - 先写 `parse_test.go`（基准 `samples/proc-example.json`）：正常驼峰 JSON `{"proc":[...]}`→[]ActualProc、按 `contact==GSEKIT_BIZ_{biz}` 过滤（nodeman 项被剔除）、空 Screen→解析失败信号、非 JSON→解析失败信号、Screen 含 "agent not available"→agent 异常信号。
   - 实现 `parse.go`：正则抽取首个 `{...}`（DOTALL）→驼峰反序列化→按 contact 过滤本业务；区分"解析失败"与"agent 异常"两类信号返回。
   - 验证：`gofmt` + `go test ./internal/processor/processcheck/...`。

3. **期望项构造（FR-006/Q-009/Q-010）— ProcName=FuncName**
   - `expected.go`：由 `Process`+`ProcessInstance`+`ProcessInfo` 调 `gse.BuildProcessOperate(BuildProcessOperateParams{Alias, FuncName=Process.Spec.FuncName, HostInstSeq, ...})` 渲染，映射为 `ExpectedProc`（valuekey=`GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}` + 9 比对字段 + ManagedStatus + 定位字段）。单进程渲染失败仅跳过该项不阻断（对标既有逐进程 continue）。
   - 验证：`gofmt` + `go build ./internal/processor/processcheck/...`。

4. **valuekey 集合 + ManagedStatus 分支（FR-005/FR-007，AC-T03/SC-006）— TDD**
   - 先写 `compare_test.go` 的非法项/分支部分：host 级 `illegal_keys = actual_keys - expected_keys`→`ILLEGAL_VALUE_KEY`；managed 无 actual→mismatch；unmanaged 有 actual→mismatch；unmanaged 无 actual→pass；starting/stopping→skip；partly_managed→ignore。
   - 实现 `compare.go` 的分支判定部分。
   - 验证：`gofmt` + `go test ./internal/processor/processcheck/...`。

5. **9 字段子集比对 + 分类（FR-006，AC-001/SC-001）— TDD**
   - 续写 `compare_test.go`：managed 且 actual 存在时，9 字段一致→pass；任一字段差异→`EXPECTATION_MISMATCH`（error_msg 含差异字段名集合）。基准用 `samples/proc-example.json` 构造一致/不一致两组。
   - 实现 `compare.go` 的子集比对：仅取 `procName/setupPath/pidPath/user/startCmd/stopCmd/restartCmd/reloadCmd/killCmd` 9 字段比对，输出每实例 `CheckResult`。
   - 验证：`gofmt` + `go test ./internal/processor/processcheck/...`。

6. **写异常/恢复决策（FR-008~FR-011，AC-001/AC-T04）— TDD**
   - 先写 `record_test.go`：fake `dao.ProcessManagedException`，验证：异常→`Create`（字段/枚举/Attachment 含 host_id=Process.Attachment.HostID）；通过且 `IsException`==true→`GetLatest`+`UpdateStatus(recovered)`；通过且非 exception→无调用；skip→无调用；`UpdateStatus` 失败仅记日志不 panic。
   - 实现 `record.go`：`CheckResult` → 上游 DAO 调用编排。
   - 验证：`gofmt` + `go test ./internal/processor/processcheck/...`。

7. **ScriptRunner seam + 单业务编排（FR-002/FR-003/FR-010/FR-012）**
   - `executor.go`：定义 `ScriptRunner` 接口（按 agentID+OS 下发 cat .proc → 返回 Screen/错误），实现体内构造 `common.Executor{GseService, GseConf:cc.G().GSE, TaskConf:cc.G().TaskFramework}` 复用 `WaitExecuteScriptFinish`。
   - `checker.go`：单业务流程——`ListProcessesWithInstance`+`GetByProcessIDs`→按 agentID 分组→（rateLimiter.Wait + 信号量并发）下发 cat .proc→解析（contact 过滤）→比对（illegal + ManagedStatus 分支 + 9 字段）→host 级错误扇出到该 agent 全部实例→逐实例落库。单进程/单 host 失败 `logs.Errorf`+continue。
   - 集成测试：fake `ScriptRunner`（各类 Screen/错误码）+ fake DAO，验证写异常/恢复与"单主机失败不阻断其余"（AC-001/AC-002/AC-003）。
   - 验证：`gofmt` + `go test ./internal/processor/processcheck/...`。

8. **定时任务入口 + 接入（FR-001/FR-013/FR-014，AC-004/AC-T01/SC-004/SC-007）**
   - `cmd/data-service/service/crontab/check_managed_process.go`：`NewCheckManagedProcess(set, sd, gseSvc, cfg)` + `Run()`（ticker + `shutdown.AddNotifier()` + `select`；`ticker.C` 分支先 `if !state.IsMaster(){continue}`）；跨业务用 `ListBizTenantMap` 逐 biz 调 checker；持有全局 `rate.Limiter`。
   - `cmd/data-service/app/app.go`：`startCronTasks()` 中 `if crontabConfig.CheckProcessManaged.Enabled { ... .Run() }`，注入 `ds.daoSet/ds.sd/ds.gseSvc`。
   - 验证：`gofmt` + `go build ./cmd/data-service/...`。

9. **测试收口**
   - 单包单测全绿：`go test ./internal/processor/processcheck/... ./pkg/cc/...`。
   - 全量编译：`go build ./...`。
   - 跨租户遍历 + 真实 GSE/DB 路径以集成 mock + 代码评审保障（research D11）。

## 验收映射（计划覆盖核对）

| AC / SC | 覆盖步骤 | FR |
|---------|---------|-----|
| AC-001 写异常字段完整 | 5(比对)+6(Create)+7(编排) | FR-005~FR-009 |
| AC-002 单主机失败不阻断 | 7(host 扇出+错误隔离) | FR-010/FR-012 |
| AC-003 / AC-T04 恢复闭环 | 6(IsException+UpdateStatus) | FR-011 |
| AC-004 / AC-T01 slave 跳过 | 8(IsMaster 守卫) | FR-001 |
| AC-T02 / SC-005 解析失败 | 2(解析)+7(扇出 PARSING_FAILED) | FR-004 |
| AC-T03 非法 valuekey + contact 过滤 | 2(过滤)+4(illegal_keys) | FR-007 |
| AC-P01 / SC-007 限流 | 8(全局 rateLimiter)+7(并发上限) | FR-014 |
| SC-001 100% 产记录 | 4+5+6 | FR-005~FR-009 |
| SC-002 失败范围准确记录 | 4(分类)+7(扇出) | FR-010 |
| SC-003 下一轮翻转 recovered | 6 | FR-011 |
| SC-004 slave 不下发 | 8 | FR-001 |
| SC-006 nodeman 不误报 + 非法项必记 | 2(contact 过滤)+4(illegal) | FR-007 |

| FR | 落点 |
|----|------|
| FR-001 定时仅 master | 步骤 8 |
| FR-002 跨租户按业务遍历 | 步骤 7(编排)+8(ListBizTenantMap) |
| FR-003 复用脚本执行构建块 | 步骤 7(executor.go) |
| FR-004 .proc 脚本可配 + 解析失败 | 步骤 1(配置)+2(解析) |
| FR-005 ManagedStatus 分支 + 枚举映射 | 步骤 4 |
| FR-006 9 字段子集比对（procName=FuncName）| 步骤 3(expected)+5(比对) |
| FR-007 contact 过滤 + valuekey + 非法项 | 步骤 2+4 |
| FR-008 异常类别映射 | 步骤 4+5 |
| FR-009 写 exception 记录 | 步骤 6 |
| FR-010 实例粒度 + host 扇出 + 错误隔离 | 步骤 6+7 |
| FR-011 恢复闭环 | 步骤 6 |
| FR-012 单点失败不阻断 | 步骤 7 |
| FR-013 巡检子配置 | 步骤 1+8 |
| FR-014 限流/并发 | 步骤 7+8 |
| FR-015 不落敏感信息 | 步骤 6(仅运维字段)+评审 |

## 复杂度 / 风险跟踪

| 项 | 说明 | 处置 |
|----|------|------|
| .proc 格式/路径兼容（TR-001）| agent 版本/部署差异 | 脚本可配（D2），解析失败按 `PARSING_FAILED` 不阻断 |
| 大规模 GSE 压力（TR-002）| 逐 host 下发 | 全局 rateLimiter + 按 agent 聚合（一次拿全 host 项）+ 信号量并发上限（D9/D10）|
| 任务重入（TR-003）| 巡检耗时叠加周期 | IsMaster 守卫 + 每轮串行 + 周期 20m 余量 |
| 期望口径与 GSE 渲染差异（TR-004）| 不调 CMDB | 以 bscp DB ProcessInfo 为准，9 字段子集比对对标 gsekit，差异归 EXPECTATION_MISMATCH 由运维确认 |
| 稳态掉管不告警（attempt-2 取舍）| syncCmdbGse 抢先刷 unmanaged | 接受取舍，由 syncCmdbGse 在控制台反映；不新增表字段（research D3a）|
| DAO 行为/真实 GSE 不可单测 | 依赖带 DB/GSE 环境 | 纯函数 + 接口 fake 覆盖核心判定；端到端以集成 + 评审（D11，plan-report testability 记录）|

## 验证命令汇总

```bash
gofmt -l pkg/cc internal/processor/processcheck cmd/data-service/service/crontab cmd/data-service/app
go test ./internal/processor/processcheck/...
go test ./pkg/cc/...
go build ./...
```
