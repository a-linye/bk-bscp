# 需求规范：进程托管配置定时检查与异常闭环

**需求 ID**：短 ID 135663906 / 长 ID 1020451610135663906
**父需求**：【bscp 进程管理】gse 托管信息检查（短 ID 135190552）
**创建日期**：2026-06-30
**状态**：Draft
**输入**：基于 `req.md` 的需求描述与 `questions.md` 中 Q-001~Q-007 的澄清结论（均为 resolved_by_doc）

## 概述

为父需求"GSE 托管信息检查"提供**核心检查引擎**：通过定时任务按业务维度逐个扫描全部进程实例，比对 bscp DB 记录的期望托管配置与 GSE 侧实际托管配置的一致性，识别异常并写入上游「托管异常记录」；当后续轮次检查恢复一致时自动将异常记录置为已恢复，形成「检查—记录—恢复」闭环。本子需求聚焦**检查与闭环编排逻辑**，复用上游子需求（#135663687）的存储能力，不含表结构定义，也不含操作拦截。

## 用户场景与测试 *(必需)*

### 用户故事 1 — 定时托管配置一致性检查与异常记录 (优先级: P0)

作为平台运维，我希望系统定时巡检全业务进程实例的 GSE 托管配置一致性，在期望配置与实际配置发生漂移时识别异常类别并写入异常记录，以便在托管配置漂移时第一时间发现、定位并按建议处置。

**优先级理由**：这是父需求检查闭环的核心能力，缺此则异常无从被发现，上游存储与下游操作拦截均无数据来源，构成最小可用切片。

**独立测试**：可单独 mock GSE 返回各类 `.proc` Screen / 错误码，对单业务遍历下的「写异常」路径与「单主机失败不阻断其余」进行独立验证；并以单元测试覆盖 `.proc` 解析与逐项比对判定。

**验收场景**：

1. **Given** 某进程实例 bscp 期望托管配置与 GSE 实际托管配置存在差异，**When** 定时检查执行，**Then** 该进程实例被判定为托管异常，并写入一条异常记录（含异常类型、差异原因、处理建议）（对应 AC-001）。
2. **Given** 某业务下部分主机/进程检查失败，**When** 定时检查执行，**Then** 失败范围被记录为异常，其余业务/进程检查不受影响继续完成（对应 AC-002）。
3. **Given** 当前实例为 slave，**When** 定时周期到达，**Then** 该实例跳过执行，不下发任何 GSE 脚本，仅 master 执行检查（对应 AC-004 / AC-T01）。
4. **Given** agent `.proc` 内容无法解析为 JSON，**When** 巡检执行，**Then** 该主机相关进程实例记 `PARSING_FAILED`，其余业务/主机检查不受影响（对应 AC-T02）。
5. **Given** GSE 实际托管项存在期望集合外的 valuekey，**When** 比对执行，**Then** 记 `ILLEGAL_VALUE_KEY` 异常（对应 AC-T03）。

---

### 用户故事 2 — 异常自动恢复（闭环） (优先级: P1)

作为平台运维，我希望已记录的异常在后续检查恢复一致时自动解除，以便异常状态可自动闭环，无需人工逐条清理。

**优先级理由**：自动恢复是异常闭环的另一半，避免运维逐条手动清理已恢复的异常，与检查写入共同构成完整业务价值。

**独立测试**：可构造"上轮 exception + 本轮一致""上轮无记录 + 本轮一致""上轮 recovered"等样本，mock 上游 DAO 的 `IsException`/`UpdateStatus`，独立验证恢复闭环判定与写入路径。

**验收场景**：

1. **Given** 某进程实例当前处于托管异常态且后续 GSE 实际配置已与期望一致，**When** 下一轮定时检查执行，**Then** 该进程实例最新异常记录状态被更新为"已恢复"（对应 AC-003 / AC-T04）。
2. **Given** 某进程实例本轮检查通过且其最新记录非 exception（无记录或已 recovered），**When** 巡检执行，**Then** 不产生多余写入或状态更新动作。
3. **Given** 状态更新（恢复）写库失败，**When** 巡检执行，**Then** 记录日志并在下一轮重试，不阻断其余进程实例检查。

---

### 边界场景

- **期望配置来源**：期望托管配置取自 bscp DB（`Process.Spec.SourceData` 反序列化为 `ProcessInfo`），本期不为渲染期望配置而调用 CMDB。
- **本业务托管项过滤**：解析 `.proc` 时按 `contact == GSEKIT_BIZ_{bizID}` 过滤本业务托管项，再以 valuekey（`GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`）与期望项逐一匹配。
- **比对子集语义**：对标 gsekit，期望项为实际项的子集即视为一致；存在差异字段则记 `EXPECTATION_MISMATCH`，并在 `error_msg` 写出差异字段集合。
- **大规模限流**：按业务分批并对 GSE 调用限流，避免对 GSE 形成压力；并发上限受控。
- **任务重入**：单实例 `IsMaster` 守卫 + 每轮串行，周期默认 20 分钟留足余量，避免巡检耗时与周期叠加导致重入。
- **agent 异常**：脚本日志含 "agent not available" 类信号 / agent 非 normal 时记 `AGENT_EXCEPTION`，不阻断其余检查。
- **数据保护**：检查与记录过程不落入敏感个人信息。

## 需求 *(必需)*

### 功能需求

- **FR-001**：系统必须在 data-service 内新增一个定时巡检任务，接入现有 crontab 框架，仅在主实例（`IsMaster()` 为真）执行。
- **FR-002**：系统必须按业务维度跨租户逐个遍历：取全部 `biz_id→tenant_id` 映射，逐业务以对应租户上下文处理；单业务内取该业务下全部进程实例及其在 bscp DB 中的期望托管配置。
- **FR-003**：系统必须复用「GSE 异步脚本执行 + 结果轮询 + Screen 解析」构建块向目标 agent 下发 `cat .proc` 脚本，从返回 Screen 解析得到 GSE 实际托管配置；不引入 istep 任务流水线抽象。
- **FR-004**：`.proc` 读取脚本内容/路径必须可配置，缺省值对标 gsekit（linux `cat /usr/local/gse2_bkte/agent/etc/.proc`、windows `type c:\gse2_bkte\agent\etc\.proc`）；解析失败按 `PARSING_FAILED` 处理且不阻断。
- **FR-005**：系统必须逐项比对期望配置与实际配置并识别异常类别，按 questions.md Q-003 的映射归入上游枚举：配置不符（已托管无信息 / 未托管有信息 / 属性差异）→ `EXPECTATION_MISMATCH`；非法托管项 → `ILLEGAL_VALUE_KEY`；获取/解析失败 → `PARSING_FAILED`；agent 异常 → `AGENT_EXCEPTION`；其余 → `OTHER`。
- **FR-006**：参与"配置属性不一致"比对的期望字段必须取 `ProcessInfo` 中与托管相关字段：`WorkPath`、`PidFile`、`StartCmd`、`StopCmd`、`RestartCmd`、`ReloadCmd`、`FaceStopCmd`、`User`，外加进程名 `FuncName`。
- **FR-007**：对判定为异常的进程实例，系统必须调用上游存储能力以追加方式写入一条 `status=exception` 的托管异常记录（含 `error_type`/`error_msg`/`handling_suggestion`/`checked_at`）。
- **FR-008**：检查粒度必须以 process_instance 为单位写记录；host 级错误（解析失败 / agent 异常）必须扇出到该主机下全部相关进程实例。
- **FR-009**：对本轮检查通过的进程实例，系统必须在其最新记录为 exception 时（`IsException`==true）调用上游 `UpdateStatus` 将最新记录置为 `recovered` 完成闭环；最新记录非 exception 时不动作。恢复判定以"最近一次检查结论"为准。
- **FR-010**：单业务 / 单主机 / 单进程检查失败必须仅记录该范围异常，不阻断其余业务的检查继续完成；状态更新失败仅记日志并在下一轮重试。
- **FR-011**：系统必须新增巡检子配置（启用开关 / 周期 / QPS 限流），默认周期 20 分钟、限流量级与现有 crontab 任务一致；任务按启用开关守卫启动，未开启时不影响既有行为。
- **FR-012**：对 GSE 的调用必须按现有限流模式（`rate.Limiter`）受限，按业务分批、并发上限受控，不对 GSE 造成异常压力。
- **FR-013**：检查与记录过程不得落入敏感个人信息。

### 范围外（本子需求不包含）

- 「托管异常记录」表结构与 DAO 定义（见子需求"进程托管异常记录数据存储" #135663687，本子需求复用）。
- 进程操作拦截逻辑（见子需求"异常托管进程操作拦截"）。
- 为渲染期望配置而调用 CMDB。
- 异常的前端展示 / 告警。
- 复用 `internal/task/executor/config/config_check.go` 的 istep step/callback 整套任务编排流水线（仅复用其 GSE 脚本执行 + Screen 解析构建块，依据 AGENTS.md 不引入不必要抽象，详见 questions.md Q-001）。

### 关键实体 *(涉及数据)*

- **进程期望托管配置（ProcessInfo，读取自 `Process.Spec.SourceData`）**：bscp DB 中记录的某进程的期望托管配置，参与比对字段见 FR-006。
- **GSE 实际托管项（解析自 agent `.proc` Screen）**：某 agent 上 GSE 实际托管的进程项集合，按 `contact`（`GSEKIT_BIZ_{bizID}`）过滤本业务、按 valuekey 匹配期望项。
- **托管异常记录（ProcessManagedException，上游 #135663687 提供）**：本子需求只读写不定义。写入业务字段 `error_type`/`error_msg`/`handling_suggestion`/`status`/`checked_at`，定位字段 `tenant_id`/`biz_id`/`host_id`/`process_id`/`process_instance_id`；恢复时更新 `status` 为 `recovered`。

## 成功标准 *(必需)*

### 可度量结果

- **SC-001**：当某进程实例期望配置与 GSE 实际配置存在差异时，巡检 100% 产出一条异常记录，且异常类型 / 差异原因 / 处理建议字段完整。
- **SC-002**：某业务部分主机/进程检查失败时，其余业务/进程检查 100% 继续完成，失败范围被准确记录为对应异常类别。
- **SC-003**：处于异常态的进程实例在后续检查恢复一致后，其异常态在下一轮巡检结束后被解除（最新记录状态翻转为 recovered）。
- **SC-004**：在 slave 实例上，巡检周期到达时不下发任何 GSE 脚本（执行被跳过）。
- **SC-005**：`.proc` 内容无法解析时，受影响主机的进程实例记 `PARSING_FAILED`，不影响其余主机/业务。
- **SC-006**：单轮巡检对 GSE 的调用始终受限流约束，不对 GSE 造成异常压力。

## 验收标准映射

| 验收编号 | 来源 | 覆盖的功能需求 / 场景 |
|---------|------|---------------------|
| AC-001 | req.md 功能验收 | FR-005~FR-008；用户故事 1 场景 1 |
| AC-002 | req.md 功能验收 | FR-010；用户故事 1 场景 2 |
| AC-003 | req.md 功能验收 | FR-009；用户故事 2 场景 1 |
| AC-004 | req.md 功能验收 | FR-001；用户故事 1 场景 3 |
| AC-P01 | req.md 性能验收 | FR-012；边界场景"大规模限流"、SC-006 |
| AC-T01 | req.md 技术澄清补充 | FR-001（IsMaster 守卫）；用户故事 1 场景 3、SC-004 |
| AC-T02 | req.md 技术澄清补充 | FR-004（解析失败）；用户故事 1 场景 4、SC-005 |
| AC-T03 | req.md 技术澄清补充 | FR-005（非法 valuekey）；用户故事 1 场景 5 |
| AC-T04 | req.md 技术澄清补充 | FR-009（恢复闭环）；用户故事 2 场景 1 |

## 假设

- 复用仓库现有 crontab 框架与样板（`sync_cmdb.go`：ticker + `shutdown.AddNotifier()` + `IsMaster()` 守卫），无新引入框架/库（依据 req.md 技术方案 / questions.md Q-004）。
- GSE 实际配置获取复用既有 `GseService`（`AsyncExtensionsExecuteScript` + `WaitExecuteScriptFinish` + Screen 解析），读取 agent `.proc` 以具备权限的执行用户（对标 gsekit `ACCOUNT_ALIAS` root/Administrator）（依据 questions.md Q-001）。
- bscp 注册 GSE 进程的 namespace 与 gsekit contact 完全一致（`GSEKIT_BIZ_{bizID}`），期望 valuekey 为 `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`（依据 questions.md Q-002）。
- 上游「托管异常记录」DAO（`Create`/`GetLatestByProcessInstanceID`/`IsException`/`UpdateStatus`）契约稳定且 #135663687 已就绪（依据 req.md 外部依赖 / questions.md Q-007）。
- 按业务跨租户遍历用 `App().ListBizTenantMap(kit)`，单业务取数沿用 `SyncSingleBiz` 样板（`ListProcessesWithInstance` + `GetByProcessIDs`）（依据 questions.md Q-005）。
- 异常类别→上游枚举映射、比对子集语义对标 gsekit `check_process.py`（依据 questions.md Q-003 / Q-006）。
- 巡检子配置最终命名（含 `.proc` 脚本配置键、QpsLimit 默认值 / 限流粒度）在 plan 阶段统一确认（依据 req.md TQ-001 / TQ-002，非阻塞）。

## 依赖

- **强依赖**：进程托管异常记录数据存储（#135663687）——本子需求读写其表结构与 DAO（`internal/dal/dao/process_managed_exception.go`），#135663687 已就绪。
- GSE 异步脚本执行 HTTP API（复用既有 `internal/components/gse`），用于读取 agent `.proc`。
- 进程内接口：主从选举 `State.IsMaster()`、biz→tenant 映射 `App().ListBizTenantMap`，均已就绪。
- 进程数据源 DAO：`Process().ListProcessesWithInstance` / `ProcessInstance().GetByProcessIDs`，均已就绪。
