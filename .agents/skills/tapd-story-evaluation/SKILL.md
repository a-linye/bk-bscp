---
name: tapd-story-evaluation
slug: tapd-story-evaluation
version: 1.0.0
description: |
  TAPD 需求评估技能。基于规范需求文档进行逻辑分析，进行子需求拆分和 RICE 价值规模评分。
  包含两个子技能：tapd-story-breakdown（需求拆分）和 tapd-story-score（价值规模评分）。
  Use this skill whenever the user mentions 需求评估, 评估需求, 需求拆分, 拆分需求,
  规模评分, RICE 评分, 价值评分, 需求规模, story evaluation, story breakdown,
  story scoring, evaluate requirement, break down story, RICE score,
  or any workflow involving TAPD story decomposition and value scoring.
metadata:
  requires:
    mcps: ["tapd"]
    skills: ["tapd-story-breakdown", "tapd-story-score"]
---

# TAPD 需求评估

## 概述

本技能对 TAPD 需求进行逻辑分析，基于接口定义、公共功能库、前端模块、后端模块逻辑
划分进行子需求拆分，拆分后输出各子需求的规范需求文档，并使用 RICE 模型进行价值
规模评分。整个流程由两个子技能协作完成：

- **tapd-story-breakdown**：负责需求拆分，输出各子需求的规范文档
- **tapd-story-score**：负责工时预估和 RICE 价值规模评分

## 前置条件

- TAPD MCP 服务可用
- 用户提供至少一个需求 ID
- workspace_id 可由用户提供，或从项目根目录 `project.json` 读取
- 支持 Windows / Linux / macOS 系统（所有文件操作均使用跨平台方式）

## 输入

| 参数 | 来源 | 必需 | 说明 |
|------|------|------|------|
| 需求 ID | 用户输入 | 是 | 一个或多个 TAPD 需求短 ID 或长 ID |
| workspace_id | 用户输入 > project.json | 是 | TAPD 工作空间 ID |
| 背景知识 | 用户指定 > AGENTS.md 自动查找 | 否 | 架构文档、模块文档、安全规范、前端规范、后端规范等路径 |

## 执行流程

### 1. 参数收集与环境准备

#### 1.1 确定 workspace_id

按以下优先级确定：
1. 用户消息中显式指定 → 直接使用
2. `project.json` 中的 `workspace_id` → 使用 `read_file` 读取并解析
3. 以上均无 → 询问用户

#### 1.2 收集背景知识

按以下优先级确定：
1. 用户显式指定背景文档路径 → 读取指定文档
2. 用户未指定 → 读取 `AGENTS.md`，从中按需查找以下文档：
   - 架构文档（如 `docs/architecture.md`）
   - 模块文档（如 `docs/modules/`）
   - 安全规范（如 `docs/security.md`）
   - 前端规范（如 `docs/frontend-guide.md`）
   - 后端规范（如 `docs/backend-guide.md`）
   - API 文档、Proto 文件等

只读取实际存在的文档，不存在的跳过。将收集到的背景知识作为后续评估的参考上下文。

#### 1.3 解析需求 ID 列表

从用户输入中提取所有需求 ID，构建待处理列表。如果 ID 长度小于 19 位，后续使用
TAPD MCP `tapd_id_get` 转换为 19 位长 ID。

### 2. 逐一处理需求

对每个需求 ID 执行以下流程（顺序执行，完成一个再处理下一个）：

#### 2.1 提取需求详情

使用 TAPD MCP `stories_get` 提取需求信息：

```
调用参数:
  workspace_id: <workspace_id>
  id: <需求ID>
  with_v_status: "1"
```

提取成功后检查 `v_status`：
- 如果 v_status 为"approved"→ 正常继续处理
- 如果 v_status 不为"approved"→ 询问用户是否跳过该需求
  - 用户选择跳过 → 继续处理下一个需求
  - 用户选择不跳过 → 继续处理当前需求

提取的关键信息：
- `id`（完整 19 位 ID）
- `name`（需求名称）
- `description`（需求描述，即规范需求文档）
- `priority_label`（优先级）
- `owner`（处理人）
- `parent_id`（父需求 ID）
- `v_status`（需求状态）

#### 2.2 需求拆分

整合背景知识与需求描述，调用子 skill `tapd-story-breakdown` 进行需求拆分。

子 skill 的详细流程见 `tapd-story-breakdown/SKILL.md`。

**拆分结果处理**：

- **无需拆分**（≤2 个用户故事）→ 跳过拆分，直接进入步骤 2.3 对原需求评分
- **有子需求输出** → 进入步骤 2.4

#### 2.3 单需求评分（无需拆分时）

如果需求不需要拆分，直接调用子 skill `tapd-story-score` 对该需求进行工时预估和
RICE 评分。评分完成后，该需求处理结束，跳过后续步骤。

子 skill 的详细流程见 `tapd-story-score/SKILL.md`。

#### 2.4 用户确认拆分结果

将拆分产出的多个子需求文档汇总展示给用户：
- 列出每个子需求的名称、核心功能点、依赖关系
- 说明拆分逻辑和理由
- 等待用户确认

**用户反馈处理**：
- 用户确认通过 → 进入步骤 2.5
- 用户确认不通过 → 收集用户的修改意见和问题，结合反馈回到步骤 2.2 重新拆分
- **终止条件**：累计 3 轮确认仍未通过时，终止拆分流程，告知用户"拆分方案无法达成共识，建议用户手动调整需求描述后重试"，跳过该需求继续处理下一个

#### 2.5 逐一评分

用户确认拆分结果后，对每个子需求调用子 skill `tapd-story-score` 进行：
1. 工时预估
2. RICE 价值规模评分
3. 将评分结果更新到子需求文档中

### 3. 创建子需求单据

对拆分产出的多个子需求，按以下**四阶段创建协议**逐一处理（顺序执行，完成一个再处理下一个）：

#### 必需字段清单

每个子需求单据必须填入以下字段（description 为需求文档全量信息，便于后续流程流转）：

| 字段 | 值来源 | 说明 |
|------|--------|------|
| workspace_id | 步骤 1.1 | 项目 ID |
| name | 子需求文件名 | 需求名称 |
| parent_id | 需求文档中的父需求 ID | 19 位长 ID，短 ID 需先用 `tapd_id_get` 转换 |
| description | 子需求文档全量内容 | 保障后续信息流转的完整信息 |
| with_v_status | 固定值 | 必填，值为 1 |
| v_status | 固定值 | 必填，值为 "approved" |
| owner | project.json 中 owner | 处理人 |
| creator | project.json 中 owner | 创建人 |
| developer | project.json 中 owner | 开发人员 |
| priority_label | 需求文档中优先级 | High / Middle / Low |
| effort | 需求文档中预估工时 | 如 "16" |
| size | 需求文档中 RICE 评分 | RICE 价值规模评分值 |
| created | 当前时间 | ISO 格式时间戳 |

#### 阶段 1 — 前置检查（Pre-check）

遍历必需字段清单，检查每个字段的值是否可从以下来源确定：子需求文档、`project.json`、步骤 1.1 已收集的上下文变量（如 workspace_id），或为固定值（`with_v_status=1`、`v_status="approved"`、`created=当前时间`）。

- 所有字段均有值 → 进入阶段 2
- 有字段缺失且值无法确定 → **阻断**：以列表形式展示缺失字段，告知用户补充后可重试，跳过该子需求，继续处理下一个

#### 阶段 2 — 创建（Create）

> ⚠️ **前置操作**：调用 `stories_create` 前，必须先通过读取 §3.4 保存的子需求本地文件（`docs/reqs/<子需求文件名>.md`）获取完整内容，将读取结果作为 `description` 参数值传入，**禁止**从上下文直接 inline。

调用 TAPD MCP `stories_create`，传入上述必需字段清单中的全部字段。若调用失败，按错误处理表格中「子需求创建失败」的处理方式执行（重试一次，仍失败则输出文档供手动创建），记录状态「❌ 创建失败」，跳过阶段 3 和阶段 4。

#### 阶段 3 — 后置验证（Post-verify）

调用 TAPD MCP `stories_get`（参数：`workspace_id`（步骤 1.1）、`id`（阶段 2 返回的 story ID）），提取实际写入的字段值，对以下业务字段逐一检验是否有非空值（`with_v_status` 和 `created` 为写入辅助参数，API 不回写，不参与验证）：`name`、`parent_id`、`description`、`v_status`、`owner`、`creator`、`developer`、`priority_label`、`effort`、`size`。

- 全部字段均有非空值 → ✅ 验证通过，记录状态「✅ 通过」
- 有字段为空或缺失 → 进入阶段 4

#### 阶段 4 — 自动修复（Fix）

由于阶段 1 已确认所有必需字段的值可确定，对每个验证缺失的字段，直接从原来源（子需求文档、`project.json`、步骤 1.1 上下文或固定值）取值，调用 TAPD MCP `stories_update` 补全（调用参数：`workspace_id`（步骤 1.1）、`id`（阶段 2 返回的 story ID）、以及各缺失字段的键值对）。调用成功，记录「🔧 已修复（{字段列表}）」；若调用失败，记录「⚠️ 修复失败（{字段列表}）」并告知用户手动补全对应字段。

> ⚠️ **注意**：从子需求文档取值时，必须通过读取本地文件 `docs/reqs/<子需求文件名>.md` 获取，**禁止**从上下文直接 inline。

每个子需求最终输出状态之一：
- `✅ 通过`：所有字段验证通过
- `🔧 已修复（{字段列表}）`：有字段缺失但已自动补全
- `⚠️ 修复失败（{字段列表}）`：stories_update 调用失败，已告知用户手动补全
- `❌ 创建失败`：stories_create 重试后仍失败，已输出文档供手动创建
- `⏭ 已跳过（前置检查缺失：{字段列表}）`：必需字段值无法确定，未创建单据

### 4. 汇总输出

所有需求处理完毕后，简短总结输出处理内容。

「单据状态」聚合规则（将该父需求下各子需求的终态汇总为一个单元格）：
- 全部为 `✅ 通过` → `✅ 全部通过`
- 有修复/失败/跳过时，逐类计数，如 `2✅ 1🔧`、`2✅ 1⏭`、`1✅ 1❌`
- 无子需求（无需拆分）→ `—`

```markdown
## 需求评估完成

| 需求 ID | 需求名称 | 处理结果 | 子需求数 | RICE 评分 | 单据状态 | 本地文件 |
|---------|---------|---------|---------|----------|---------|---------|
| xxx | xxx | ✅ 已拆分并评分 | 3 | 80/65/45 | ✅ 全部通过 | docs/reqs/xxx.md |
| aaa | aaa | ✅ 已拆分并评分 | 3 | 70/50/40 | 2✅ 1⏭ | docs/reqs/aaa.md |
| yyy | yyy | ✅ 无需拆分，已评分 | 0 | 120 | — | docs/reqs/yyy.md |

共处理 N 个需求，拆分产出 M 个子需求，已创建 K 个 TAPD 子需求单据。
```

## 错误处理

| 错误场景 | 处理方式 |
|---------|---------|
| TAPD MCP 不可用 | 终止执行，提示用户检查 MCP 配置 |
| 需求 ID 不存在 | 跳过该需求，继续处理下一个 |
| 需求状态不是"approved" | 提示用户，询问是否仍要评估 |
| 子需求创建失败 | 重试一次，仍失败则输出文档供手动创建 |
| 前置检查发现必需字段缺失 | 阻断该子需求，以列表形式展示缺失字段，告知用户补充后可重试，继续处理下一个 |
| 后置验证发现字段缺失 | 从原来源自动补全（调用 stories_update），记录「🔧 已修复」；若补全失败则告知用户 |
| project.json 不存在 | 询问用户提供 workspace_id 和 owner |
| 背景知识文档不存在 | 跳过不存在的文档，使用已有信息继续 |

## 子技能

本 Skill 包含两个子技能，各自独立运作，由主流程编排调用：

| 子技能 | 路径 | 功能 |
|--------|------|------|
| tapd-story-breakdown | `tapd-story-breakdown/SKILL.md` | 需求拆分 |
| tapd-story-score | `tapd-story-score/SKILL.md` | 工时预估与 RICE 评分 |

## 参考文件

| 文件 | 用途 | 何时读取 |
|------|------|---------|
| `references/requirement-splitting-guide.md` | 需求拆分原则与方法 | 执行需求拆分时 |
| `references/rice-scoring-standard.md` | RICE 模型评分标准 | 执行价值评分时 |
| `references/requirement-doc-template.md` | 子需求文档模板 | 生成子需求文档时 |

## 产出

- 拆分后的子需求文档（如发生拆分），保存在 `docs/reqs/` 目录
- 包含工时预估和 RICE 评分的需求文档
- 创建的 TAPD 子需求单据（v_status 为"approved"）
- 处理汇总报告

## 使用示例

```
用户输入：评估需求 12345

系统处理：
1. 获取 workspace_id，收集项目背景知识
2. 使用 TAPD MCP 获取需求 12345 详情
3. 分析需求包含 4 个用户故事，需要拆分
4. 调用 tapd-story-breakdown 拆分为 3 个子需求
5. 展示拆分结果，等待用户确认
6. 用户确认后，逐一调用 tapd-story-score 评分
7. 按四阶段协议（前置检查→创建→后置验证→自动修复）创建 3 个子需求单据
8. 输出评估汇总
```

```
用户输入：评估需求 67890，这个需求比较简单

系统处理：
1. 获取 workspace_id，收集项目背景知识
2. 使用 TAPD MCP 获取需求 67890 详情
3. 分析需求只有 1 个用户故事，无需拆分
4. 直接调用 tapd-story-score 评分
5. 输出评估结果
```
