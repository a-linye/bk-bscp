# 同步 CMDB agent_id 失败

## 基本信息

| 字段 | 值 |
|------|-----|
| 需求 ID | 1020451610136462061（短 ID 136462061） |
| 需求名称 | 同步 cmdb agentid 失败 |
| 优先级 | High |
| 父需求 | 无 |
| 创建时间 | 2026-07-24 11:28:21 |
| 原始需求文档 | docs/reqs/agentid同步修复.md |
| 预估工时 | 16 人时（2 人天） |
| 价值规模（RICE） | 100 |

## 需求背景

### 业务背景

bscp 进程配置管理模块会把 CMDB 的进程/主机信息同步到本地，并依赖主机的 `agent_id` 调用 GSE 查询进程运行状态（running/stopped、托管状态）。

当前存在以下问题：进程首次被 bscp 同步落库时，如果该主机在 CMDB 中尚未就绪（`bk_agent_id` 为空，常见于 agent 刚安装、尚未注册的窗口期），bscp 会把空 `agent_id` 固化到本地记录；之后即便 CMDB 补齐了 `agent_id`，无论全量同步还是增量事件都不会再刷新本地的 `agent_id`，导致该字段长期为空。

连带问题：同步过程中 `agent_status` 会被刷新为 `normal`（该字段会更新），但 `agent_id` 仍为空，形成"`agent_status=normal` 且 `agent_id` 为空"的不一致组合。该组合不会被全量同步的可同步过滤跳过，反而持续用空 `agent_id` 调用 GSE 并报错（如 `gse API error code=1015500`），使进程运行状态始终无法同步。

痛点与影响：

- 新增进程在页面上看似 agent 正常，但进程运行状态、状态获取时间长期为空。
- 空 `agent_id` 触发的 GSE 调用必然失败，产生持续的失败任务与错误日志。
- `agent_status` 与 `agent_id` 数据不一致，误导问题排查。

目标用户：使用 bscp 进程配置功能的业务运维/研发。使用场景：在 CMDB 中新增进程后，通过 bscp 查看与管理进程状态。

不做的影响：新增进程只要在"agent 未就绪"窗口内被创建，`agent_id` 将永久为空，进程状态无法同步，且持续产生无效 GSE 调用与失败任务。

### 用户故事

作为 bscp 进程配置的使用者，
我希望新增进程后 bscp 本地记录的 `agent_id` 能与 CMDB 保持一致（在 CMDB 补齐 `agent_id` 后自动刷新），
以便进程状态能够正常同步、页面数据准确。

作为平台维护者，
我希望不再出现"`agent_status=normal` 但 `agent_id` 为空"的不一致数据，
以便避免无效的 GSE 调用与失败任务堆积。

### 需求来源

- **需求渠道**：线上问题 / 技术优化
- **关联需求**：无
- **参考资料**：本次线上排查结论（biz 5000079，进程 683，host 10.99.135.145）

## 功能需求

### 核心功能点

| 功能编号 | 功能描述 | 优先级 | 涉及角色 | 备注 |
|---------|---------|--------|---------|------|
| F-001 | 同步时刷新本地 `agent_id` 使其与 CMDB 一致 | P0 | 系统 | 必须 |
| F-002 | `agent_id` 为空时强制 `agent_status=abnormal` | P0 | 系统 | 必须 |
| F-003 | 存量空 `agent_id` 数据经同步自动纠正 | P1 | 系统 | 应该有 |

### 详细功能描述

#### [F-001] agent_id 随同步刷新

- **输入**：CMDB 全量同步 / 增量进程事件同步
- **处理逻辑**：
  1. 从 CMDB 拉取到的最新 `agent_id` 与本地记录不一致时，更新本地 `agent_id`。
  2. 全量同步与增量事件两条入口均需支持刷新（不再只在进程首次创建时写入一次）。
- **输出**：本地 `processes.agent_id` 与 CMDB 的 `host.bk_agent_id` 保持一致。
- **边界条件**：CMDB 返回空 `agent_id` 时按 F-002 处理。
- **异常处理**：CMDB 查询失败时保持本地原值，不得误清空。

#### [F-002] agent_id 与 agent_status 一致性兜底

- **输入**：进程同步落库 / 更新
- **处理逻辑**：当 `agent_id` 为空时，`agent_status` 一律置为 `abnormal`。
- **输出**：系统内不存在"`agent_status=normal` 且 `agent_id` 为空"的组合。

#### [F-003] 存量数据自动纠正

- **输入**：修复上线后的全量 / 增量同步
- **处理逻辑**：对已存在的空 `agent_id` 进程，同步时按 F-001 刷新为 CMDB 最新值。
- **输出**：存量脏数据在一次或多次同步后被纠正，无需单独的一次性数据订正脚本。

## 非功能需求

### 性能需求

- 不引入额外的显著性能开销，同步耗时维持在现有量级。

### 安全需求

- 不改变现有权限模型与数据可见性。

### 可用性与稳定性

- 修复后减少空 `agent_id` 触发的无效 GSE 调用与失败任务，降低错误日志量。

### 兼容性

- 不改变对外接口契约，仅修正内部同步行为。

## 业务规则

### 业务逻辑规则

- **规则 R-001**：本地 `agent_id` 以 CMDB 的 `bk_agent_id` 为准；二者不一致时以 CMDB 为准刷新。
- **规则 R-002**：`agent_id` 为空 ⇒ `agent_status = abnormal`。
- **规则 R-003**：CMDB 查询失败时不覆盖本地已有 `agent_id`，避免误清空。

### 数据校验规则

- **相关字段**：`processes.agent_id`、`processes.agent_status`
- **取值范围**：`agent_status` ∈ {`normal`, `abnormal`}

## 外部依赖与集成

### 外部系统集成

| 系统名称 | 交互方式 | 接口说明 | 认证方式 | 文档链接 |
|---------|---------|---------|---------|---------|
| CMDB | HTTP | `process_related_info` 提供 `host.bk_agent_id` | APIGW | 内部 |
| GSE | HTTP | 依赖 `agent_id` 查询进程运行/托管状态 | APIGW | 内部 |

### 数据模型

- `processes.agent_id`：主机 agent 标识，来源 CMDB `host.bk_agent_id`。
- `processes.agent_status`：agent 状态，取值 `normal`/`abnormal`。

## 验收标准

> 验收口径：字段级——以 `processes.agent_id` 与 CMDB 保持一致为准。

### 功能验收

- [ ] **AC-001**：Given 某进程本地 `agent_id` 为空且 CMDB 已有有效 `bk_agent_id`，When 执行全量同步，Then 本地 `processes.agent_id` 更新为 CMDB 的 `bk_agent_id`。
- [ ] **AC-002**：Given 同 AC-001 条件，When 触发该进程的增量 CMDB 事件同步，Then 本地 `agent_id` 更新为 CMDB 值。
- [ ] **AC-003**：Given 某进程 CMDB 返回空 `bk_agent_id`，When 同步落库，Then 本地 `agent_status=abnormal`，且系统内不出现"`normal` + 空 `agent_id`"组合。
- [ ] **AC-004**：Given 存量 `agent_id` 为空的进程且 CMDB 已补齐 `agent_id`，When 执行同步，Then 该进程 `agent_id` 被自动纠正为 CMDB 值。
- [ ] **AC-005**：Given CMDB `agent_id` 查询失败，When 同步执行，Then 本地已有 `agent_id` 不被清空。

## 边界范围

### 本期包含

- 全量同步与增量事件同步刷新 `agent_id`
- `agent_id` 为空时 `agent_status` 兜底为 `abnormal`
- 存量空 `agent_id` 经同步自动纠正

### 本期不包含

- 单独的一次性数据订正脚本（通过同步自动纠正达成）
- CMDB 侧 agent 注册 / 数据质量问题（属 CMDB 范畴）
- 进程状态同步（GSE）逻辑的其他优化
- 端到端"进程运行状态同步成功"的保证（本期验收以字段级 `agent_id` 一致为准）

## 价值规模评分

### 人力与工时

- 全量工作 1 位高级工程师完成工时预估：16 人时（2 人天）
- 全量工作 1 位中级工程师完成工时预估：约 24 人时（高级工程师的 1.5 倍）

工时构成（高级工程师基准，含开发 + 自测 + 代码审查 + 文档）：

- diff 逻辑改造（`BuildProcessChanges` 纳入 `agent_id` 变更判定并更新 `attachment.agent_id`，覆盖全量与增量事件两条入口）：约 6 人时
- 一致性兜底（`agent_id` 为空 ⇒ `agent_status=abnormal`）及 CMDB 查询失败不清空保护：约 2 人时
- 单元测试（agent_id 刷新、空值兜底、查询失败保护、存量纠正等用例）：约 6 人时
- 边界核对与验证（全量/增量路径回归、字段一致性）：约 2 人时

### RICE 评分明细

| 参数 | 值 | 说明 |
|------|-----|------|
| Reach | 20 | 影响进程配置模块使用者中在"agent 未就绪"窗口内新增的进程 |
| Impact | 10 | 修复线上缺陷（进程状态无法同步、持续无效 GSE 调用） |
| Confidence | 100% | 根因已定位并验证，方案明确，需求文档完善 |
| Effort | 2 | 预估 16 人时 = 2 人天 |

**RICE Score = (20 × 10 × 1.0) / 2 = 100** → 🟠 高（优先安排，尽快进入开发）

## 约束条件

- **技术限制**：遵循仓库 Go 规范；改动小、边界清晰、可验证。

## 未解决问题

| 问题 ID | 问题描述 | 截止日期 | 状态 |
|--------|---------|---------|------|
| Q-001 | 存量纠正是否需要观测/统计被纠正的进程数量以便验证效果 | 待定 | 待确认 |

---

## 原需求描述

> (无描述内容)

## 澄清记录

### 第 1 轮澄清

**提问时间**：2026-07-24 11:31

**Agent 提问**：

1. 本需求的范围界定为哪个？
2. 对已经产生的 `agent_id` 为空的存量进程，期望怎么处理？
3. `agent_id` 应通过哪些同步入口刷新到最新？
4. 是否要求 `agent_id` 为空时强制把 `agent_status` 置为 `abnormal`？
5. 本需求的验收口径以哪个为准？

**用户回复**：

1. 核心缺陷修复 + `agent_id`/`agent_status` 一致性治理。
2. 修复后让全量/增量同步能自动纠正存量数据。
3. 全量同步 + 增量事件同步都要能刷新。
4. 是，强制兜底：`agent_id` 为空则 `agent_status=abnormal`。
5. 字段级：`processes.agent_id` 与 CMDB 保持一致即可。
