---
name: tapd-story-commit
slug: tapd-story-commit
version: 2.0.0
description: |
  迭代执行流水线提交阶段。在主会话中直接执行（不走 subagent），负责：
  变更统计、成本数据汇总、构建 commit 信息、git 提交、TAPD 状态更新。
  是 pipeline 中唯一在主会话执行的子 skill。
---

# 需求实现提交

## 定位

commit 是 pipeline 的**终结阶段**——代码已通过全部校验（架构/安全/CodeReview），
本阶段负责"收尾"：统计成果、记录度量、提交代码、更新外部状态。

> 本子 skill 在主会话中直接执行，不通过 speckit-executor-agent。
> 原因：commit 操作需要完整的 git 权限和 TAPD MCP 访问，且无 speckit 命令可调用。

## 前置条件

- 当前需求 `meta.yaml.phase` 为 `validated`
- 代码已通过架构校验、安全校验和 CodeReview
- `meta.yaml.implement_baseline_commit` 已存在（implement 阶段写入）

## 输入

- `${WORKDIR}/meta.yaml`（读取 `implement_baseline_commit` / `stats.cost` / `history` / `workspace_id`）
- `${WORKDIR}/context.md`（Code scope 白名单，用于确认变更范围）

## 执行流程

### 1. 变更统计

统计本需求从 implement 开始到当前的**全量代码变更**。

**基线**：`meta.yaml.implement_baseline_commit`
**对比目标**：当前工作区（HEAD）

**统计维度**：

| 指标 | 说明 |
|------|------|
| `total` | 总变更行数（新增 + 删除） |
| `add_code` | 新增行数 |
| `delete_code` | 删除行数 |
| `logic_code` | 非测试、非文档的代码文件变更行数 |
| `test_code` | 测试文件变更行数（`*_test.*` / `*_spec.*` / `test_*.*`） |
| `docs` | 文档文件变更行数（`*.md` / `*.txt` / `*.rst`） |
| `files` | 变更文件数 |

使用 `git diff --numstat <baseline>` 获取原始数据，按文件后缀归类。

### 2. 成本数据汇总

从 `meta.yaml.stats.cost` 读取已由 pipeline 主编排实时累加的成本度量数据，汇总两个维度。

#### 3.1 总体 cost

直接读取 `cost` 的顶层 total 字段：

| 指标 | 来源字段 |
|------|---------|
| 总耗时 | `cost.total_duration_sec`（秒） |
| 总成本 | `cost.total_credit`（积分；同 `cost.total_cost_usd`） |
| 总输入 tokens | `cost.total_input_tokens` |
| 总输出 tokens | `cost.total_output_tokens` |
| 总缓存 tokens | `cost.total_cache_tokens` |
| subagent 调用次数 | `cost.subagent_calls` |

#### 3.2 各阶段 cost

遍历 `cost.per_call[]`，按 `stage` 字段分组聚合（duration_sec / credit / input_tokens / output_tokens / cache_tokens / calls），将结果写入 `meta.yaml.stats.cost.per_stage`。

stage 取值：`clarify` / `specify` / `plan` / `tasks-generate` / `tasks-analyze` / `implement` / `validate-arch` / `validate-security` / `validate-codereview` / `validate-fix`

### 3. 更新 meta.yaml.stats

将步骤 2 的代码变更统计写入 `meta.yaml.stats`（与已有的 `cost` 字段并列）：

```yaml
stats:
  total: <TOTAL>
  add_code: <ADD_CODE>
  delete_code: <DELETE_CODE>
  logic_code: <LOGIC_CODE>
  test_code: <TEST_CODE>
  docs: <DOCS>
  files: <FILES>
  cost:
    # ... 已有字段不变，新增 per_stage ...
    per_stage: { ... }
```

> `started_at` / `end_at` 不由本子 skill 维护——开始时间在 `history[0].ts`，
> 结束时间在最新 `history` 条目的 `ts`。

### 4. 更新状态

1. 更新 `meta.yaml.phase` 为 `committed`
2. `meta.yaml.history` 追加成功记录，清空 `meta.yaml.last_failure`

### 5. 构建 Commit 信息

按 `../references/commit-conventions.md` 规范构建 commit message：

- **type**：feat / fix / refactor / docs / test 等
- **scope**：涉及的模块
- **subject**：一句话概括变更目的（祈使语气）
- **body**：变更内容要点
- **footer**：`--story=${ID}`（关联 TAPD 需求）

### 6. 记录 commit.md

将 commit 信息保存到 `${WORKDIR}/commit.md`，格式如下：

```markdown
# Commit 记录

## Commit Message

<构建好的 commit message>

## Commit Hash

<git rev-parse HEAD 的输出>

## 变更统计

| 指标 | 值 |
|------|-----|
| 总变更行数 | <total> |
| 新增代码 | <add_code> |
| 删除代码 | <delete_code> |
| 逻辑代码 | <logic_code> |
| 测试代码 | <test_code> |
| 文档变更 | <docs> |
| 变更文件数 | <files> |

## 成本汇总

### 总体

| 指标 | 值 |
|------|-----|
| 总耗时 | <total_duration_sec> s |
| 总成本 | <total_credit> credit |
| 总输入 tokens | <total_input_tokens> |
| 总输出 tokens | <total_output_tokens> |
| 总缓存 tokens | <total_cache_tokens> |
| subagent 调用次数 | <subagent_calls> |

### 各阶段

| 阶段 | 耗时 | 成本 | 输入 tokens | 输出 tokens | 缓存 tokens | 调用次数 |
|------|------|------|------------|------------|------------|---------|
| <stage> | <duration_sec> s | <credit> credit | <input_tokens> | <output_tokens> | <cache_tokens> | <calls> |
| ... | | | | | | |

## 时间

- 开始时间：<history[0].ts>
- 完成时间：<当前 ISO 8601 时间>
```

### 7. 提交代码

```
git add -A
git commit -m "<commit message>"
```

> commit后任何文件信息回填与补充都无需commit，会有其他流程处理。

### 8. 更新 TAPD 需求状态

使用 TAPD MCP `stories_update` 更新需求状态：

| 参数 | 值 | 来源 |
|------|-----|------|
| workspace_id | TAPD 工作空间 ID | `meta.yaml.workspace_id` → `project.json` → 询问 |
| id | 需求 ID | pipeline 传入 |
| v_status | "for test" | 固定值 |

> **错误处理**：`stories_update` 失败时**不阻塞** git commit（代码提交是核心操作）。
> 失败信息记录到 `process.log`，由人工后续在 TAPD 中补录。

### 9. 更新 meta.yaml.phase

将 `meta.yaml.phase` 更新为 `committed`。

## 可重入约定

commit 是 pipeline 终结阶段，通常不会被回退到。但以下场景需要幂等保证：

| 场景 | 处理 |
|------|------|
| git commit 因 hooks 失败 | 修复后重试，`git add -A && git commit` 幂等 |
| TAPD 更新失败 | 不阻塞，记录日志后正常推进 phase |
| 重复进入已是 `committed` 的 phase | pipeline 主编排检测到 `phase==committed` 直接退出（完工）|
| 中途崩溃后恢复 | meta.yaml 已是 `validated` → 重新执行全部步骤（幂等：git add -A 收集相同内容，commit message 重新生成）|

## 参考资料

| 文件 | 用途 | 何时读取 |
|------|------|---------|
| `../references/commit-conventions.md` | Commit message 格式规范 | 步骤 6 构建 commit 信息时 |
| `../references/context-and-meta-template.md` | meta.yaml stats 字段定义 | 步骤 4 写入统计时 |

## 产出

- 代码已提交至本地仓库（git commit）
- `${WORKDIR}/meta.yaml` — stats 字段已填充（代码变更 + cost 汇总）
- `${WORKDIR}/commit.md` — Commit 记录文件（含变更统计 + 成本汇总 + 时间）
- TAPD 需求 v_status 更新为"for test"
- 需求 phase 为 `committed`
