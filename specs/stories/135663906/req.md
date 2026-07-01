# 进程托管配置定时检查与异常闭环

## 基本信息

| 字段 | 值 |
|------|-----|
| 需求 ID | 1020451610135663906 |
| 需求名称 | 进程托管配置定时检查与异常闭环 |
| 父需求 | 【bscp 进程管理】gse托管信息检查 |
| 父需求ID | 父需求短ID：135190552 / 长ID：1020451610135190552 |
| 父需求文档 | docs/reqs/GSE托管信息检查.md |
| 优先级 | High |
| 价值规模 | 32（Reach=20, Impact=8, Confidence=100%, Effort=5人天） |
| 预估工时 | 40 人时 |
| 创建时间 | 2026-06-30 16:35:00 |
| 原始需求文档 | docs/reqs/进程托管配置定时检查与异常闭环.md |

## 依赖关系

> 有依赖。

| 依赖类型 | 依赖需求 | 需求ID | 说明 |
|---------|---------|------|------|
| 强依赖 | 进程托管异常记录数据存储 | 长ID：1020451610135663687 | 检查结论需写入「托管异常记录」表并更新恢复状态，依赖其表结构与 DAO |

## 需求背景

### 业务背景

bscp DB 记录的进程托管配置与 GSE 侧实际托管配置之间可能出现漂移（外部直接改动、托管/取消托管中断、非法托管项等）。本子需求是父需求的**核心检查引擎**：通过定时任务按业务逐个扫描全部进程实例，比对 bscp 期望托管配置与 GSE 实际托管配置的完整一致性，识别异常并写入异常记录；当后续检查恢复一致时自动解除异常态，形成检查—记录—恢复的闭环。

聚焦检查与闭环逻辑，复用上游子需求的存储能力，不含表结构定义，也不含操作拦截。

### 用户故事

作为 **平台运维**
我想要 **系统定时巡检全业务进程的 GSE 托管配置一致性并记录异常**
以便于 **在托管配置发生漂移时第一时间发现、定位并按建议处置**

作为 **平台运维**
我想要 **已记录的异常在后续检查恢复一致时自动解除**
以便于 **异常状态可自动闭环，无需人工逐条清理**

## 功能需求

### 核心功能点

| 功能编号 | 功能描述 | 优先级 | 涉及角色 | 备注 |
|---------|---------|--------|---------|------|
| F-001 | 定时任务按业务维度逐个扫描全部进程实例，比对 GSE 实际托管配置与 bscp 期望托管配置的一致性，写入异常记录 | P0 | 平台后台 | 完整配置比对，对标 gsekit |
| F-002 | 下一轮检查通过的进程实例，其异常记录自动解除（恢复正常） | P1 | 平台后台 | 自动闭环 |

### 详细功能描述

#### [F-001] 定时托管配置一致性检查

- **输入**：定时触发（周期性）。检查对象为按业务维度遍历到的全部进程实例。
- **处理逻辑**：
  1. 接入现有 crontab 框架，主从选举：仅 master 实例执行。
  2. 按业务维度逐个处理：取该业务下全部进程实例及其在 bscp DB 中的期望托管配置（ProcessInfo：工作路径、PID 文件、启动/停止/重启/重载/强制停止命令、启动用户等）。
  3. 获取 GSE 侧实际托管配置：复用现有“配置检查”机制（internal/task/executor/config/config_check.go），通过 GSE 异步脚本执行接口（AsyncExtensionsExecuteScript / async_execute_script + get_execute_script_result）向目标 agent 下发脚本读取 agent 的 .proc 文件（对标 gsekit cat .proc），从返回 Screen 解析得到实际托管配置。
  4. 逐项比对期望配置与实际配置，识别异常类别（对标 gsekit）：已托管但 GSE 无托管信息 / 未托管但 GSE 有托管信息 / 非法托管项 / 配置属性不一致 / 获取解析失败、agent 异常。
  5. 对判定为异常的进程实例，调用上游存储能力写入「托管异常记录」。
- **输出**：每个进程实例的检查结论与对应的异常记录写入。
- **边界条件**：
  - 期望配置取自 bscp DB，本期不为渲染期望配置而调用 CMDB。
  - 大规模时按业务分批并对 GSE 调用限流（参考现有 rateLimiter）。
- **异常处理**：
  - 单业务/单主机检查失败 → 记录该范围异常，不阻断其余业务检查。
  - GSE 调用失败 → 记录为对应异常类别，给出排查方向。

#### [F-002] 异常自动恢复（闭环）

- **输入**：后续轮次 F-001 的检查结论。
- **处理逻辑**：当某进程实例在后续检查中恢复一致（检查通过），调用上游存储能力将其异常记录状态更新为“已恢复”。
- **输出**：异常态被解除，操作侧不再拦截。
- **边界条件**：以“最近一次检查结论”为准。
- **异常处理**：状态更新失败记录日志，下一轮重试。

## 非功能需求

### 性能需求
- **执行周期**：周期性执行，周期可配置；默认值待确认（建议默认 20 分钟，对齐 sync_cmdb）。
- **并发能力**：按业务分批；对 GSE 调用按现有 rateLimiter 模式限流，避免对 GSE 形成压力。

### 安全需求
- **数据保护**：检查与记录过程不落入敏感个人信息。

## 验收标准

### 功能验收

- [ ] **AC-001**：Given 某进程实例 bscp 期望托管配置与 GSE 实际托管配置存在差异 When 定时检查执行 Then 该进程实例被判定为托管异常，并写入一条异常记录（含异常类型、差异原因、处理建议）。
- [ ] **AC-002**：Given 某业务下部分主机/进程检查失败 When 定时检查执行 Then 失败范围被记录为异常，其余业务/进程检查不受影响继续完成。
- [ ] **AC-003**：Given 某进程实例当前处于托管异常态且后续 GSE 实际配置已与期望一致 When 下一轮定时检查执行 Then 该进程实例异常记录状态被更新为“已恢复”。
- [ ] **AC-004**：Given 当前实例为 slave When 定时周期到达 Then 该实例跳过执行，仅 master 执行检查。

### 性能验收

- [ ] **AC-P01**：定时检查按业务逐个扫描，单轮检查对 GSE 的调用受限流约束，不对 GSE 造成异常压力（具体阈值随规模补充后确定）。

## 边界范围

### 本子需求包含
- 定时按业务扫描全部进程实例的完整托管配置一致性检查（F-001）。
- 异常自动恢复闭环（F-002）。

### 本子需求不包含
- 「托管异常记录」表结构与 DAO 定义（见子需求“进程托管异常记录数据存储”，本子需求复用）。
- 进程操作拦截逻辑（见子需求“异常托管进程操作拦截”）。
- 为渲染期望配置而调用 CMDB。
- 异常的前端展示/告警。

## 人力与工时

* 全量工作1位高级工程师完成工时预估：40 人时
* 全量工作1位中级工程师完成工时预估：56 人时

## RICE 评分明细

| 参数 | 值 | 说明 |
|------|-----|------|
| Reach | 20 | 影响进程管理模块/特定角色 |
| Impact | 8 | 核心功能：主动发现托管配置漂移 |
| Confidence | 100% | 技术路径已明确：复用配置检查机制，经 GSE 异步脚本执行接口读取 agent .proc 文件（Q-001 已确定） |
| Effort | 5 | 预估 40 人时 = 5 人天（crontab 接入 + 按业务扫描 + GSE 实际配置获取 + 完整比对 + 写异常 + 自动恢复 + 测试） |
| **RICE Score** | **32** | 🟢 低：工时占比偏高，建议在数据基础就绪后排期推进 |

## 技术澄清

> 澄清日期：2026-06-30
> 需求复杂度：中等偏复杂
> 澄清轮次：1
> 自答依据：context.md 白名单文档（详见 questions.md Q-001 ~ Q-007）

### 技术审查结论

- **技术可行性**：✅ 可行。检查—记录—恢复闭环所需的全部构建块（GSE 异步脚本执行、crontab+主从选举、按业务遍历、上游异常记录 DAO）均已在仓库存在，本需求为「编排 + 比对」而非新基建。
- **技术风险等级**：中。主要风险集中在 .proc 解析的格式兼容与大规模下的 GSE 调用压力，已有限流/容错样板可控。

### 技术方案概述

- **实现方式**：在 `cmd/data-service/service/crontab/` 新增一个定时巡检任务（沿用 `sync_cmdb.go` 样板：`ticker` + `shutdown.AddNotifier()` + `IsMaster()` 守卫），按业务遍历进程实例，复用「GSE 异步脚本执行」读取 agent `.proc`，逐项比对 bscp 期望托管配置与 GSE 实际托管配置，将异常结论写入上游「托管异常记录」并在恢复时闭环。
- **涉及模块**：
  - `cmd/data-service/service/crontab/`：新增巡检任务（启动/周期/IsMaster 守卫/按业务遍历/限流）。
  - `cmd/data-service/app/app.go`：在 `startCronTasks()` 中按新配置项 `Enabled` 守卫启动。
  - `pkg/cc/types.go` + `pkg/cc/service.go` + `cmd/data-service/etc/data_service.yaml`：新增巡检配置项（Enabled/Interval/QpsLimit + 默认值/校验 + 样例）。
  - GSE 实际配置获取：复用 `internal/components/gse`（`AsyncExtensionsExecuteScript`/`GetExecuteScriptResult`）与 `internal/task/executor/common` 的 `WaitExecuteScriptFinish` 轮询样板；新增 `cat .proc` 脚本构造与 Screen→JSON 解析逻辑。
  - 数据源：`internal/dal/dao/app.go` `ListBizTenantMap`、`internal/dal/dao/process.go` `ListProcessesWithInstance`、`internal/dal/dao/process_instance.go` `GetByProcessIDs`。
  - 写异常/恢复：复用上游 `internal/dal/dao/process_managed_exception.go`（`Create`/`GetLatestByProcessInstanceID`/`IsException`/`UpdateStatus`）。
- **技术选型**：不新引入框架/库。明确**不复用** `internal/task/executor/config/config_check.go` 的 istep step/callback 整套任务流水线，仅复用其 GSE 脚本执行 + Screen 解析的构建块，避免引入与本巡检无关的任务编排抽象（符合 AGENTS.md「不引入不必要抽象」）。

### 架构影响

- **新增组件**：data-service 内一个新的 crontab 巡检任务（按业务全量托管配置一致性检查器）。
- **变更组件**：`startCronTasks()` 注册逻辑、`CrontabConfig` 新增子配置、`data_service.yaml` 新增样例。
- **数据模型变更**：无（不新增表/字段；「托管异常记录」表由上游 #135663687 提供，本需求仅读写）。
- **向后兼容性**：兼容。新巡检任务默认可配置开关（`Enabled`），未开启时不影响既有行为；纯新增配置项，旧配置缺省走默认值。

### 外部依赖

| 依赖项 | 类型 | 状态 | 接口/文档 | 备注 |
|--------|------|------|---------|------|
| GSE 异步脚本执行 | HTTP API | ✅ 已确认 | `internal/components/gse/script.go`（async_execute_script / get_execute_script_result） | 复用既有 `GseService`；读取 agent `.proc` 需以具备权限的执行用户（对标 gsekit `ACCOUNT_ALIAS` root/Administrator） |
| 上游托管异常记录 DAO | 进程内 DAO | ✅ 已确认（#135663687 已就绪） | `internal/dal/dao/process_managed_exception.go` | Create/GetLatest/IsException/UpdateStatus 契约稳定 |
| 主从选举 | 进程内接口 | ✅ 已确认 | `internal/serviced/serviced.go` `State.IsMaster()` | 仅 master 执行 |
| biz→tenant 映射 | 进程内 DAO | ✅ 已确认 | `internal/dal/dao/app.go` `ListBizTenantMap`（SkipTenantFilter 跨租户） | 跨租户遍历数据源 |

### 技术风险

| 风险 ID | 风险描述 | 影响 | 概率 | 应对措施 |
|---------|---------|------|------|---------|
| TR-001 | agent `.proc` 文件路径/格式随 GSE agent 版本或部署环境变化，导致 `cat .proc` 失败或字段对不上 | 中 | 中 | `.proc` 读取脚本内容可配置（对标 gsekit `GlobalSettings.CHECK_PROC_SCRIPT`，以 gsekit 默认路径为缺省）；解析失败按 `PARSING_FAILED` 记录、不阻断 |
| TR-002 | 大规模业务下逐主机下发脚本对 GSE 形成压力 | 中 | 中 | 按业务分批 + `rate.NewLimiter` 限流（对标 `sync_biz_host.go`），并发上限受控（对标 `sync_gse.go` 信号量） |
| TR-003 | 巡检耗时长，与 ticker 周期叠加导致任务重入 | 低 | 中 | 单实例 `IsMaster` 守卫 + 每轮串行；周期默认 20m，留足余量；按需后续加运行中标志位 |
| TR-004 | 期望配置取自 bscp DB（不调 CMDB），与 GSE 实际渲染口径存在差异 | 中 | 低 | 本期明确口径以 bscp DB `ProcessInfo` 为准，子集比对对标 gsekit；差异如属预期渲染差，归 `EXPECTATION_MISMATCH` 由运维确认 |

### 技术决策记录

| 决策 | 选择方案 | 备选方案 | 选择理由 |
|------|---------|---------|---------|
| GSE 实际配置获取方式 | 直接复用 GSE 脚本执行构建块（AsyncExtensionsExecuteScript + WaitExecuteScriptFinish + Screen 解析） | 复用 config_check.go 整套 istep 流水线 | 巡检无需 step/callback 任务编排，直连更轻量，避免不必要抽象 |
| 异常类别落库映射 | 复用上游 5 枚举（PARSING_FAILED/AGENT_EXCEPTION/ILLEGAL_VALUE_KEY/EXPECTATION_MISMATCH/OTHER），配置不符三态统一归 EXPECTATION_MISMATCH | 新增更细分枚举 | 上游表枚举已对标 gsekit，复用即可；差异细节落 error_msg |
| 检查粒度 | 以 process_instance 为单位写记录；host 级错误（解析/agent）扇出到该主机全部实例 | 以 host 为单位记录 | 上游异常表主键定位到 process_instance_id；操作侧按实例判定异常 |
| 配置接入 | `CrontabConfig` 新增子配置（Enabled/Interval/QpsLimit），默认 20m | 硬编码周期 | 与现有 crontab 任务一致，可开关、可调周期 |

### 测试策略

- **单元测试**（优先，可单包验证）：
  - .proc Screen → 期望/实际配置对象的解析（含空内容、非 JSON、含非法 valuekey、含非本业务 contact 的过滤）。
  - 比对逻辑：已托管无信息 / 未托管有信息 / 属性差异 / 非法托管项 / 一致 五类判定及对应 `error_type`。
  - 异常→记录映射与恢复闭环判定（mock 上游 DAO：IsException + Create / UpdateStatus 调用路径）。
- **集成测试**：mock `GseService` 返回各类 Screen/错误码，验证单业务遍历下的写异常/恢复与「单主机失败不阻断其余」。
- **端到端测试**：非本期重点（依赖真实 GSE 环境）；以集成 mock 覆盖关键场景。
- **测试数据**：构造 `Process.Spec.SourceData`(ProcessInfo JSON) + `ProcessInstance` 样本与对应 .proc Screen 样本。

### 补充的验收标准

- [ ] **AC-T01**：Given 当前实例非 master When 巡检周期到达 Then 任务跳过，不下发任何 GSE 脚本（对应 AC-004，复用 IsMaster 守卫）。
- [ ] **AC-T02**：Given agent `.proc` 内容无法解析为 JSON When 巡检执行 Then 该主机相关进程实例记 `PARSING_FAILED`，其余业务/主机检查不受影响（对应 AC-002）。
- [ ] **AC-T03**：Given GSE 实际托管项存在期望集合外的 valuekey When 比对执行 Then 记 `ILLEGAL_VALUE_KEY` 异常。
- [ ] **AC-T04**：Given 某进程实例上轮为 exception 且本轮检查一致 When 巡检执行 Then 该实例最新异常记录被 `UpdateStatus` 置为 recovered（对应 AC-003）。

### 待解决问题

| 问题 ID | 问题描述 | 负责人 | 截止日期 | 状态 |
|---------|---------|--------|---------|------|
| TQ-001 | `.proc` 读取脚本内容/路径建议做成配置项，缺省值对标 gsekit；最终缺省与配置键命名在 plan 阶段确认 | 研发 | plan 阶段 | ✅ 已决策（配置化，gsekit 缺省），命名待 plan |
| TQ-002 | 新巡检配置项 `QpsLimit` 默认值与限流粒度（每业务/全局）在 plan 阶段结合现有 rateLimiter 用法定稿 | 研发 | plan 阶段 | ⚠️ 待确认（非阻塞） |

### DoR 自评（中等/复杂级）

- 技术方案明确：✅（复用既有构建块，路径清晰）
- 外部依赖已识别且可用：✅（GSE API / 上游 DAO / IsMaster / biz-tenant 映射均就绪）
- 测试策略已定义：✅（单元为主 + 集成 mock）
- 部署约束明确：✅（纯新增配置项，默认可开关；data-service 内运行，仅 master 执行）
- 回滚方案就绪：✅（关闭新增配置项 `Enabled` 即停用巡检，不影响其它链路）

结论：DoR 全部满足，可进入 specify/plan。无阻塞性 open 问题。

---

## 技术澄清补充（第 2 轮 / attempt-2）

> 澄清日期：2026-06-30
> 触发：confirm 卡点评审中，针对「期望托管态来源」「比对字段」「.proc 真实格式」做深度核对，校正首轮 spec 的若干粗略表述。
> 自答/决策依据：源码核实（见 questions.md Q-008~Q-011）+ 用户拍板 + 真实 `.proc` 样例。

### 关键决策 1：以 `ProcessInstanceSpec.ManagedStatus` 作为「是否应托管」基准（不引入期望状态字段）

bscp **不存在** gsekit `is_auto` 那种独立的「期望托管」字段；最接近且唯一可用的是 `ProcessInstanceSpec.ManagedStatus`（`pkg/dal/table/process_instance.go`）。它由 `syncCmdbGse` 定时任务周期性从 GSE 同步回本地（`docs/reqs/进程状态同步修复.md` / `进程状态同步优化.md` 明确「周期性同步 status、managed_status」），即 **bscp 侧最新已知托管态**。检查直接以它为基准：

| `ManagedStatus` | 含义 | 检查判定 |
|---|---|---|
| `managed`（托管中）| 应托管 | GSE `.proc` 必须有该 valuekey 且配置一致；无 → `EXPECTATION_MISMATCH`（已托管但未获取到信息）；属性不符 → `EXPECTATION_MISMATCH` |
| `unmanaged`（未托管）| 不应托管 | GSE `.proc` 不应有该 valuekey；有 → `EXPECTATION_MISMATCH`（未托管却有信息）；无 → 正常**不记录** |
| `starting`（托管中过渡）/ `stopping`（取消托管过渡）| 操作进行中 | 本轮**跳过**，避免操作窗口误报 |
| `""`（空，未同步过）| — | 同 `unmanaged` |
| `partly_managed` | 仅进程维度，实例上不出现 | 忽略 |

- 即把 gsekit 的 `is_auto`（True/False）直接替换为 `ManagedStatus`（managed/unmanaged），其余比对算法完全对标 gsekit `check_process.py`。
- 已知取舍：若某实例被非法掉管后 `syncCmdbGse` 抢先把 `ManagedStatus` 刷为 `unmanaged`，该「稳态掉管」当轮不告警，由 `syncCmdbGse` 自身在控制台反映；检查的核心价值（**配置属性漂移** + **非法托管项**）不受此影响。本期接受该取舍，不为此新增表字段。

### 关键决策 2：valuekey 与「期望项集合」构造

- 期望 valuekey = `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`（`gse.BuildNamespace` + `gse.BuildProcessName(alias, hostInstSeq)`，注意用进程**别名 alias**，非 func_name）。
- host 级 `expected_keys` = 该主机上**全部** bscp 进程实例的 valuekey（不区分 managed/unmanaged，只要 bscp 认识该实例）。
- `illegal_keys = actual_keys - expected_keys` → `ILLEGAL_VALUE_KEY`（host 级）。
- 对 `expected_keys` 内的每个实例，再按决策 1 的 `ManagedStatus` 分支逐一判定。

### 关键决策 3：参与比对的字段集合（对照真实 `.proc` 定稿）

真实 `.proc` 为**驼峰命名 JSON**，字段含 `procName/setupPath/pidPath/contact/startCmd/stopCmd/restartCmd/reloadCmd/killCmd/versionCmd/healthCmd/type/cpulmt/memlmt/user/password/userPwd/valuekey/startCheck*/opTimeOut/operateType/timestamp`。

bscp 期望项**只构造并比对以下 9 个字段**（bscp 经 `BuildProcessOperate` 实际下发的字段），按「期望项 ⊆ 实际项」子集比对（对标 gsekit `proc.items() <= actual.items()`）：

| `.proc` 字段（驼峰）| bscp 来源 |
|---|---|
| `procName` | `Process.Spec.FuncName`（**注意：不在 `ProcessInfo` 内**）|
| `setupPath` | `ProcessInfo.WorkPath`（渲染后）|
| `pidPath` | `ProcessInfo.PidFile`（渲染后）|
| `user` | `ProcessInfo.User`（`.proc` 中确有 user，纳入比对）|
| `startCmd` | `ProcessInfo.StartCmd`（渲染后）|
| `stopCmd` | `ProcessInfo.StopCmd`（渲染后）|
| `restartCmd` | `ProcessInfo.RestartCmd`（渲染后）|
| `reloadCmd` | `ProcessInfo.ReloadCmd`（渲染后）|
| `killCmd` | `ProcessInfo.FaceStopCmd`（渲染后）|

- **必须剔除** `versionCmd`/`healthCmd`（bscp 不下发，样例中也为空）以及 `type/cpulmt/memlmt/password/userPwd/startCheck*/opTimeOut/operateType/timestamp`（GSE agent 内部字段，bscp 不下发不关心）。若把这些纳入期望项，子集比对会恒误判。
- 差异时把差异字段名集合写入 `error_msg`。

### 关键决策 4：`.proc` 本业务过滤

- `.proc` 含多来源托管项（样例中 `contact=GSEKIT_BIZ_100148` 为 bscp/gsekit 托管，`contact=nodeman` 为节点管理插件）。
- 解析后**必须按 `contact == GSEKIT_BIZ_{bizID}` 过滤**，仅保留本业务托管项再参与匹配与比对，否则 `nodeman` 等其它来源项会被误判为 `ILLEGAL_VALUE_KEY`。
