---
name: tapd-story-govern-pipeline
slug: tapd-story-govern-pipeline
version: 1.0.0
description: |
  TAPD 需求整理流水线。按 phase 调度需求澄清、需求评审、需求评估三个独立 skill，
  将单个需求推进到可进入下一阶段的 approved 状态。Use this skill whenever the user mentions
  需求整理流水线, refinement pipeline, 需求前置编排, 需求澄清加评审加拆单, or any workflow
  involving orchestration of clarification, review, and evaluation before implementation.
metadata:
  requires:
    mcps: ["tapd"]
    os: ["linux", "macos", "windows"]
    skills: ["tapd-story-clarification", "tapd-story-evaluation", "tapd-story-review"]
---

# TAPD 需求整理流水线

## 1. 定位

本 skill 负责把**一个 TAPD 需求**从初始状态推进到“前置整理完成、可进入下一阶段”。

它本质上是一个**phase 调度器**，只做三件事：

1. 维护自己的 `meta.yaml`
2. 根据 `phase` 选择要调用的 skill
3. 根据 skill 返回结果决定推进、停住、回退还是退出

它**不把业务逻辑塞进子 skill**，也不要求子 skill 彼此感知：

- `tapd-story-clarification` 只负责澄清
- `tapd-story-evaluation` 只负责评估 / 拆单 / 评分
- `tapd-story-review` 只负责发起评审、读取评论、给出结论

**pipeline 自治原则**：

- 唯一状态契约：`${WORKDIR}/meta.yaml`
- 调用方不直接修改 `meta.yaml`
- 外部对内容文件的修改仅限本文 §10 允许的范围
- pipeline 启动时会校验状态与内容文件一致性，违规直接写 `last_failure.type=mutation_invalid` 退出
- pipeline 只负责“读状态 → 选 skill → 落地结果”，不把编排语义塞进子 skill

## 2. 入参（按优先级回退）

### 2.1 输入参数

| 参数 | 来源 | 必需 | 说明 |
|------|------|------|------|
| 父需求 ID | 调用方传入 / 用户消息 / 交互询问 | 是 | 支持短 ID 或 19 位长 ID；启动后需同时归一化出 `${SHORT_ID}` 与 `${LONG_ID}`，其中 `${SHORT_ID}` 统一取 `${LONG_ID}` 后 8 位 |
| workspace_id | 调用方传入 / `meta.yaml.workspace_id` / 用户消息 / `project.json.workspace_id` / 交互询问 | 是 | TAPD 工作空间 ID |
| reviewers | 调用方传入 / `meta.yaml.reviewers` / 用户消息 / 交互询问 | 是 | 评审人列表，使用 `@用户名` 格式 |
| action | 调用方传入 / 用户语义映射 | 否 | `execute` / `answer` / `retry` / `abort`，默认 `execute` |
| workdir | 调用方传入 / 默认派生 | 否 | 默认 `docs/reqs/${SHORT_ID}/`，内部统一记为 `${WORKDIR}` |
| req_file | 调用方注入 / TAPD 拉取生成 | 否 | 默认 `${WORKDIR}/req.md`，内部统一记为 `${REQ_FILE}` |

### 2.2 参数解析优先级

| 内部变量 | 解析顺序 |
|----------|----------|
| `${ID}` | 1. 调用方传入；2. 用户消息；3. 交互询问 |
| `${SHORT_ID}` | 1. 调用方显式传入；2. 由 `${ID}` 归一化得到；3. `meta.yaml.short_id` |
| `${LONG_ID}` | 1. 调用方显式传入；2. 由 `${ID}` 归一化得到；3. `meta.yaml.long_id` |
| `${WORKDIR}` | 1. 调用方传入；2. 默认 `docs/reqs/${SHORT_ID}/` |
| `${WORKSPACE_ID}` | 1. 调用方传入；2. `meta.yaml.workspace_id`；3. 用户消息；4. `project.json.workspace_id`；5. 交互询问 |
| `${REVIEWERS}` | 1. 调用方传入；2. `meta.yaml.reviewers`；3. 用户消息；4. 交互询问 |
| `${ACTION}` | 1. 调用方传入；2. 用户语义映射；3. 默认 `execute` |
| `${REQ_FILE}` | 1. 调用方注入到 `${WORKDIR}/req.md`；2. 不存在则通过 TAPD MCP `stories_get` 拉取父需求 description 并写入 |

> pipeline 首次执行（`meta.yaml` 不存在）时，除 `${WORKSPACE_ID}` / `${REVIEWERS}` 外，还必须把 `${SHORT_ID}` / `${LONG_ID}` 落入 `meta.yaml.short_id` / `meta.yaml.long_id`。当输入是 19 位长 ID 时，`${SHORT_ID}` 统一按 `${LONG_ID}` 后 8 位计算。后续单次调起优先从 `meta.yaml` 读取，无需调用方每次重复注入。

### 2.3 用户语义映射

| 用户表达 | 映射到 `${ACTION}` |
|----------|-------------------|
| "启动需求整理 #ID" / "跑需求整理流水线" / "继续" | `execute` |
| "我已经回答了问题" / "答案补好了" | `answer` |
| "重试" / "我改好了" | `retry` |
| "停止" / "放弃" | `abort` |

### 2.4 用户表达的具体含义

#### `execute`

适用于“正常往前推进”的场景，不代表“现在一定已经可以开始评审”，而是表示：

- 首次启动整个需求整理流水线
- 在等待评审期间，重新检查 TAPD 状态和最新评论
- 在某个阶段已处理完成后，让流水线按当前 `phase` 继续往下跑

典型理解：

- `"启动需求整理 #ID"`：首次启动整个整理流程
- `"跑需求整理流水线"`：首次启动或重新进入当前流程
- `"继续"`：不是人工审批指令，而是“请按当前 `phase` 继续调度”

例如：

- 当前 `phase=clarification-reviewing` 时，`继续` 的意思是“继续检查澄清评审是否有新结果”
- 当前 `phase=evaluation-pending` 时，`继续` 的意思是“继续执行需求评估 / 拆单”

#### `answer`

适用于流水线已经因为 `blocked` 卡点退出，且 `questions.md` 中存在待确认问题时。

这里的“我已经回答了问题 / 答案补好了”指的是：

- 需求处理人、开发人员或用户已经补充了系统提出的问题答案
- 这些问题可能来自：
  - 澄清阶段发现的信息缺失
  - 评审评论中存在业务取舍，系统无法自动判断，需要人工确认

它**不等于**“评审人已经评论了”。

更准确地说：

- 评审人已经评论了，但系统还没处理这些评论 → 用 `execute`
- 系统读完评论后，把待确认问题写入 `questions.md`，你补完答案 → 用 `answer`

#### `retry`

适用于上一次执行已经失败，当前处于 `fail` 卡点时。

这里的“重试 / 我改好了”指的是：

- 上一次不是正常等待评审，也不是正常提问，而是执行失败
- 用户已经修正了导致失败的问题，希望重新跑一次

常见场景：

- TAPD 数据有误，现已修正
- `req.md` 或其他内容文件已人工补充
- `refinement-patches/attempt-${N}.md` 已写好，希望按指定 `target_phase` 回退后重试

它**不等于**“评审意见我已经处理好了”。
如果是评审意见要求重做，通常是 pipeline 先回退 `phase`，后续仍然使用 `execute` 继续推进。

#### `abort`

适用于明确放弃当前需求整理流程。

常见场景：

- 当前需求先不做了
- 评审分歧过大，决定人工线下处理
- 当前轮次不再继续，希望流水线直接退出

### 2.5 动作选择速查

| 当前情况 | 应使用的动作 | 说明 |
|---------|-------------|------|
| 第一次启动需求整理 | `execute` | 启动整个流水线 |
| 正在等评审人评论，想再检查一次 | `execute` | 继续读取 TAPD 状态和评论 |
| 评审人已经评论，但系统还没处理这些评论 | `execute` | 让 review skill 重新读取并判断 |
| 系统已经把待确认问题写入 `questions.md`，你补完答案了 | `answer` | 表示“问题我答完了，请继续” |
| 上一次执行失败，你已经修正失败原因 | `retry` | 用于 `fail` 卡点恢复 |
| 不想继续当前流程 | `abort` | 终止当前需求整理 |

## 3. 工作目录结构

```
${WORKDIR}/
├── req.md                     # 父需求 description 快照
├── questions.md               # 待用户确认的问题（四态）
├── meta.yaml                  # 流水线状态机（唯一契约）
├── clarification-report.md    # 澄清摘要（可选）
├── evaluation-report.md       # 评估摘要（可选）
├── process.log                # 流式日志
└── refinement-patches/
    └── attempt-${N}.md        # 可选回退说明
```

`tapd-story-review` 的评审产物固定保存在：

`docs/reqs/<父需求短ID>/<phase>-review-meta.yaml`

以及：

- `docs/reqs/<父需求短ID>/clarification-review.md`
- `docs/reqs/<父需求短ID>/evaluation-review.md`

> 若 `${WORKDIR}` 目录不存在，pipeline 自动创建（`mkdir -p`）。

## 4. meta.yaml 字段定义

流水线级状态文件，保存在 `${WORKDIR}/meta.yaml`。

建议至少包含以下字段：

```yaml
short_id: "32139656"
long_id: "1070046748132139656"
story_id: "1070046748132139656"
workspace_id: "20000001"
phase: "initialized"
reviewers:
  - "@alice"
  - "@bob"
attempts: 0
round: 0
pending_review: null
last_failure: null
last_review_verdict: null
```

常用字段：

| 你需要… | 读取字段 |
|---------|---------|
| 判断当前所处阶段 | `phase` |
| 判断是否等待评审 | `pending_review` |
| 判断是否有失败 | `last_failure` |
| 判断是否有待答复问题 | `questions.md` 中的 `[open]` |
| 读取评审人 | `reviewers` |
| 读取 TAPD 工作空间 ID | `workspace_id` |

字段语义、卡点判定优先级、外部可修改文件范围分别见 §5、§9、§10。

## 5. phase 状态链

```
initialized
  → clarification-reviewing
  → evaluation-pending
  → evaluation-reviewing
  → approved
```

字段语义：

| phase | 含义                     | 下一次 `execute` 调用什么 |
|------|------------------------|---------------------------|
| `initialized` | 尚未完成澄清，或澄清评审要求重做       | `tapd-story-clarification` |
| `clarification-reviewing` | 澄清结果已发起评审，等待 review 结论 | `tapd-story-review` |
| `evaluation-pending` | 可执行评估 / 拆单，或拆单评审要求重做   | `tapd-story-evaluation` |
| `evaluation-reviewing` | 拆单结果已发起评审，等待 review 结论 | `tapd-story-review` |
| `approved` | 前置整理完成，可进入下一阶段         | 无 |

## 6. 语义指令集

| 指令 | 语义 | 适用卡点 | 调用方需提前做的事 |
|------|------|---------|-----------------|
| `execute` | 首次调度或按当前 `phase` 继续推进 | 无卡点 / waiting_review | 确保 `${ID}`、`${WORKSPACE_ID}`、`${REVIEWERS}` 可解析 |
| `answer` | 已补充系统提出的问题答案，继续推进 | blocked | 修改 `questions.md`（[open] → [answered]） |
| `retry` | 执行失败后的恢复重试 | fail | 可选：修改 `req.md` / 写 `attempt-${N}.md` |
| `abort` | 放弃当前需求整理 | 任意 | 无 |

## 7. 单次执行流程

```
1. 加载 meta.yaml（不存在 + action=execute → 按入参初始化为 phase=initialized）
2. 解析 action 指令：
   execute → 首次初始化已完成；非首次无变更
   answer  → 校验 questions.md 无 [open]，round+1
   retry   → last_failure=null, attempts+1；若有 attempt-md 则按目标 phase 回退
   abort   → 写 last_failure.type=user_aborted，退出
3. 启动轻量校验：
   req.md 存在、questions.md 格式合法、reviewers 非空
   不合法 → 写 last_failure.type=mutation_invalid 退出
4. 根据 phase 调度 skill：
   initialized              → tapd-story-clarification        → clarification-reviewing
   clarification-reviewing  → tapd-story-review               → evaluation-pending / initialized / blocked / waiting_review
   evaluation-pending       → tapd-story-evaluation           → evaluation-reviewing
   evaluation-reviewing     → tapd-story-review               → approved / evaluation-pending / blocked / waiting_review
   approved                 → 退出（完工）
5. 落盘 meta.yaml，输出退出报告
```

其中：

- 调用 TAPD 相关 skill 时，优先传 `${LONG_ID}`
- 生成本地路径、读取 review 详情文件与 review-meta 时，统一使用 `${SHORT_ID}`
- 当 `action=answer` 且当前 phase 为 `*-reviewing` 时，应将 `${WORKDIR}/questions.md` 中当前
  phase、当前 round 的 `[answered]` 条目作为 `answered_questions` 一并传给 `tapd-story-review`

**推进到下一个卡点才退出** 的规则如下：

- 若 `clarification-reviewing` 得到 `approved`，pipeline 会继续进入 `evaluation-pending`，并在同一次 `execute` 中继续调度 `tapd-story-evaluation`
- 若 `evaluation-pending` 执行成功，pipeline 会立即发起 `evaluation` 阶段评审并进入 `evaluation-reviewing`
- 只有遇到 `waiting_review`、`blocked`、`fail`、`abort` 或 `phase=approved` 时，pipeline 才退出
- 若 review 返回 `needs_rework`，pipeline 会回退 phase，但不会丢弃当前阶段对应的 review 详情文件与 `review-meta.yaml`

## 8. phase 详细行为

### 8.1 `initialized`

执行 `tapd-story-clarification`。

返回 `ok` 时：

1. 刷新 `${WORKDIR}/req.md`
2. 立即调用 `tapd-story-review(mode=request-review, phase=clarification)`
3. `phase` → `clarification-reviewing`
4. `pending_review.kind` → `external_tapd_review`
5. 退出为 `waiting_review`

返回 `blocked` 时：

- 将问题写入 `questions.md`
- 退出为 `blocked`

返回 `fail` 时：

- 写 `last_failure`
- 退出为 `fail`

### 8.2 `clarification-reviewing`

执行 `tapd-story-review(mode=process-feedback, phase=clarification)`。

根据 review 结论处理：

| review 结论 | 流水线动作 |
|-------------|-----------|
| `approved` | `phase` → `evaluation-pending`，继续推进 |
| `waiting_review` | 保持 `phase=clarification-reviewing`，退出为 `waiting_review` |
| `needs_rework` | `phase` → `initialized`，保留本轮 review 详情与保护边界，退出为 `waiting_review` |
| `blocked` | 将 review 返回的结构化待确认问题映射写入 `questions.md`，退出为 `blocked` |

> `needs_rework` 的含义是“需要重跑当前阶段处理 skill”，不是由 review skill 自己去修订。下一轮重跑 `tapd-story-clarification` 时，必须把 `${WORKDIR}/req.md` 与澄清评审详情文件中最近一轮的“基线快照 / 评审反馈摘要 / 修订保护边界”一并作为修订参考，避免把既有澄清内容整段覆盖丢失。

### 8.3 `evaluation-pending`

执行 `tapd-story-evaluation`。

返回 `ok` 时：

1. 立即调用 `tapd-story-review(mode=request-review, phase=evaluation)`
2. `phase` → `evaluation-reviewing`
3. `pending_review.kind` → `external_tapd_review`
4. 退出为 `waiting_review`

返回 `blocked` 时：

- 将问题写入 `questions.md`
- 退出为 `blocked`

返回 `fail` 时：

- 写 `last_failure`
- 退出为 `fail`

### 8.4 `evaluation-reviewing`

执行 `tapd-story-review(mode=process-feedback, phase=evaluation)`。

根据 review 结论处理：

| review 结论 | 流水线动作 |
|-------------|-----------|
| `approved` | `phase` → `approved`，退出为 `completed` |
| `waiting_review` | 保持 `phase=evaluation-reviewing`，退出为 `waiting_review` |
| `needs_rework` | `phase` → `evaluation-pending`，保留本轮评估评审详情与保护边界，退出为 `waiting_review` |
| `blocked` | 将 review 返回的结构化待确认问题映射写入 `questions.md`，退出为 `blocked` |

> 下一轮重跑 `tapd-story-evaluation` 时，必须把 `${WORKDIR}/req.md` 与评估评审详情文件中最近一轮的“父单/子单基线快照 / 评审反馈摘要 / 修订保护边界”一并作为修订参考，避免重建子需求时丢失上一轮已确认内容。

## 9. 卡点判定优先级

pipeline 退出后，调用方按以下顺序读取 `meta.yaml` 和 `questions.md`：

1. `phase == approved` → pipeline 完工
2. `last_failure` 非空 → `fail`
3. `questions.md` 有 `[open]` → `blocked`
4. `pending_review` 非空 → `waiting_review`
5. 否则 → pipeline 退出异常

### 9.1 `review blocked` 到 `questions.md` 的映射

当 `tapd-story-review` 返回 `verdict=blocked` 时，pipeline 必须把其结构化待确认问题清单稳定落盘到
`${WORKDIR}/questions.md`。每个问题至少包含以下字段：

- `question_id`
- `round`
- `scope`
- `blocking`
- `question`
- `options`
- `recommended_option`
- `answer`

推荐写法如下：

```markdown
## clarification-review Round 2

- [open] question_id: R2-Q1
  scope: 父单 #12345
  blocking: yes
  question: 是否保留旧入口兼容逻辑？
  options:
    - A: 保留一个版本周期
    - B: 立即移除
  recommended_option: A
  answer:
```

用户补充答案后，调用方只允许将 `[open]` 改为 `[answered]` 并填写 `answer` 字段；下次以
`action=answer` 调起时，pipeline 应只提取当前 phase、当前 round 的 `[answered]` 条目，映射为
`answered_questions` 传给 `tapd-story-review`。

## 10. 内容文件修改权限

`meta.yaml` 由 pipeline 独占写入，但以下内容文件允许调用方修改：

| 文件 | 外部允许修改 | 修改时机 | 说明 |
|------|------------|---------|------|
| `req.md` | ✅ | `retry` 前 | 父需求内容人工修正 |
| `questions.md`（答复字段） | ✅ | `answer` 前 | 仅修改 [open] → [answered] |
| `clarification-report.md` / `evaluation-report.md` | ✅ | `retry` 前 | 可补充人工说明 |
| `refinement-patches/attempt-${N}.md` | ✅ | `retry` 前 | 说明回退原因、目标阶段、修订建议 |
| `process.log` | ❌ | — | 审计日志，只追加 |

## 11. `retry` 的回退规则

如存在 `refinement-patches/attempt-${N}.md`，建议使用以下格式：

```yaml
target_phase: initialized
reason: review-conflict
notes:
  - 澄清范围有误，需要重做澄清
```

允许回退到的阶段：

- `initialized`
- `evaluation-pending`

回退原则：

- 只能回退到当前链路上的前序 phase
- 不允许跨阶段跳跃
- 回退后由 pipeline 在下次 `execute` 中继续调度

## 12. 与外部的通信契约

### 12.1 退出报告格式

无论调用方是用户还是未来的 runner，退出前都输出：

```
Refinement Pipeline 已退出
- 需求 ID: ${ID}
- 工作目录: ${WORKDIR}
- 当前 phase: <phase>
- 卡点类型: <approved | waiting_review | blocked | fail | abort>
- 下一步指令: <execute | answer | retry | abort>
- 说明: <简要描述当前状态>
```

不要输出 `process.log` 全量内容，不要在退出报告中重复展开内部状态细节；退出后由调用方或用户决策下一步。

### 12.2 子 skill 传参与独立性

本 pipeline 只把必要的业务参数传给子 skill：

- `tapd-story-clarification`：`${LONG_ID}`、`${WORKSPACE_ID}`；若为 review 返工重跑，还应附带 `${WORKDIR}/req.md` 和最近一轮 clarification review 快照作为背景知识
- `tapd-story-review`：`${LONG_ID}`、`${WORKSPACE_ID}`、`${REVIEWERS}`、`phase`、`mode`；若当前由 `blocked` 经 `answer` 恢复，还应附带 `answered_questions`
- `tapd-story-evaluation`：`${LONG_ID}`、`${WORKSPACE_ID}`；若为 review 返工重跑，还应附带 `${WORKDIR}/req.md` 和最近一轮 evaluation review 快照作为背景知识

它不向子 skill 注入编排语义，也不要求子 skill 感知 pipeline。

### 12.3 退出报告用途

退出报告面向用户阅读；若未来由外部调度器或 agent 驱动本 pipeline，调度方不应依赖退出报告文案做决策，而应从 `meta.yaml` 和 `questions.md` 读取结构化状态。

### 12.4 与未来前期调度器的协作

当未来由统一的前期治理 agent 调度本 pipeline 时，建议遵循以下契约：

- 调度方传入 `${ID}` / `${SHORT_ID}` / `${LONG_ID}` / `${WORKDIR}` / `${WORKSPACE_ID}` / `${REVIEWERS}`
- 调度方可在首次启动前预写 `${WORKDIR}/req.md`，也可由 pipeline 自行从 TAPD 拉取生成
- pipeline 首次执行时把 `${WORKSPACE_ID}` / `${REVIEWERS}` 落入 `meta.yaml`
- 调度方在 `needs_rework` 后不得丢弃对应 phase 的 review 详情文件与 `review-meta.yaml`，下一轮重跑必须保留最近一轮内容
- pipeline 每次执行完一段退出后，调度方只读 `${WORKDIR}/meta.yaml` 与 `questions.md` 决策下一步
- 调度方不直接修改 `meta.yaml`

### 12.5 用户独立模式速查表

| 用户说 | 映射到 action |
|--------|--------------|
| "启动需求整理 #ID" / "跑需求整理流水线" | `execute` |
| "继续" / "继续检查评审" | `execute` |
| "我已经回答了问题" | `answer` |
| "重试" / "我改好了" | `retry` |
| "停止" / "放弃" | `abort` |

### 12.6 契约版本

本契约基于 `tapd-story-refinement-pipeline v1.0.0`。如后续新增 `phase`、动作语义或卡点类型，需同步更新本 SKILL 和相关调度文档。

## 13. 错误处理

| 错误场景 | 处理方式 |
|---------|---------|
| TAPD MCP 不可用 | 终止执行，提示用户检查 MCP 配置 |
| 父需求不存在 | 写 `last_failure.type=semantic`，退出 |
| 无法解析 `workspace_id` | 交互询问；仍缺失则退出 |
| `reviewers` 为空 | 交互询问；仍缺失则退出 |
| review skill 返回 `blocked`（含冲突问题 / 待确认问题） | 写入 `questions.md`，退出为 `blocked` |
| clarification / evaluation skill 失败 | 写 `last_failure`，退出为 `fail` |
| 外部非法修改 `meta.yaml` | 写 `last_failure.type=mutation_invalid`，退出 |

## 14. 参考文件

| 文件 | 用途 |
|------|------|
| `../tapd-story-clarification/SKILL.md` | 需求澄清阶段执行规范 |
| `../tapd-story-evaluation/SKILL.md` | 拆单与工时评估阶段执行规范 |
| `../tapd-story-review/SKILL.md` | 需求评审阶段执行规范 |
| `../tapd-story-pipeline/references/state-mutation-guide.md` | 参考现有 pipeline 的状态写入边界 |

## 15. 与后续 pipeline 的关系

当本 pipeline 到达 `approved` 后，需求具备以下条件：

- 父单经过澄清并完成评审
- 若需要拆单，父单和子单的评估结果已完成评审
- 关键业务问题已在前置阶段收敛

此时可以把父单或子单交给 `tapd-story-pipeline` 进入实现流程。

## 16. Example: Happy Path

用户输入："启动需求整理 12345，评审人 @alice @bob"

**第 1 次调起：**

1. 初始化 `${WORKDIR}=docs/reqs/12345/`
2. 调 `tapd-story-clarification`
3. 调 `tapd-story-review(mode=request-review, phase=clarification)`
4. `phase=clarification-reviewing`
5. 退出，等待评审人评论

**第 2 次调起：**

1. 用户说“继续”
2. 调 `tapd-story-review(mode=process-feedback, phase=clarification)`
3. review 结论为 `approved`
4. `phase=evaluation-pending`
5. 继续调 `tapd-story-evaluation`
6. 调 `tapd-story-review(mode=request-review, phase=evaluation)`
7. `phase=evaluation-reviewing`
8. 退出，等待拆单评审

**第 3 次调起：**

1. 用户说“继续”
2. 调 `tapd-story-review(mode=process-feedback, phase=evaluation)`
3. review 结论为 `approved`
4. `phase=approved`
5. 退出（完工）
