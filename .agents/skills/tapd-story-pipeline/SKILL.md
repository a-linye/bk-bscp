---
name: tapd-story-pipeline
slug: tapd-story-pipeline
version: 1.1.0
description: |
  单需求实现流水线——把一个 TAPD 需求从零推进到代码提交。自动串联技术澄清、
  开发计划、任务拆分、TDD 实现、架构/安全校验、代码提交六个阶段。
  Use this skill whenever the user mentions 需求实现, 实现需求 #ID, 开发需求,
  TDD 开发单个需求, story pipeline, single story, 独立需求, 紧急需求, bug 修复,
  hotfix, or any single-story development workflow — even if the user just says
  "帮我实现这个需求" or "开发 #12345".
metadata:
  requires:
    mcps: ["tapd", "git"]
    os: ["linux", "macos", "windows"]
---

# 单需求实现流水线

## 1. 定位

本 skill 把**一个 TAPD 需求**从 `initialized` 状态推进到 `committed`。它既是迭代调度器
`tapd-iteration-runner` 的"被调用函数"，也是用户在迭代之外处理紧急需求 / bug /
独立轻量需求时的直接入口。

**pipeline 自治原则**：

- 不创建 / 切换 git 分支——始终在调用方所在的当前分支工作
- 不读 `iteration-state.json`——无迭代概念
- 唯一状态契约：`${WORKDIR}/meta.yaml`
- 外部对状态文件的修改必须遵循 `references/state-mutation-guide.md`
- pipeline 启动时严格校验外部修改合法性，违规直接写
  `meta.yaml.last_failure.type=mutation_invalid` 退出

## 2. 入参（按优先级回退）

| 入参 | 来源 |
|------|------|
| `${ID}` | 1. 调用方传入；2. 用户消息；3. 交互询问 |
| `${WORKDIR}` | 1. 调用方传入；2. 默认 `specs/stories/${ID}/`（独立模式）或 `specs/${VERSION}/${ID}/`（runner 调度）|
| `${WORKSPACE_ID}` | 1. 调用方传入（runner 内联执行时作为入参注入 / 用户独立模式可显式给出）；2. `meta.yaml.workspace_id`；3. 用户消息；4. `project.json.workspace_id`；5. 交互询问 |
| `${REQ_FILE}` | 1. 调用方注入到 `${WORKDIR}/req.md`；2. 不存在则通过 TAPD MCP `stories_get` 拉取并写入，参数 `id=${ID}`、`workspace_id=${WORKSPACE_ID}` |
| `${AGENT_TOOL}` | 1. 调用方传入；2. `meta.yaml.agent_tool`；3. 用户消息；4. 默认 `agent` |

> pipeline 首次执行（`meta.yaml` 不存在）时，把 `${WORKSPACE_ID}` / `${AGENT_TOOL}` 落入 `meta.yaml.workspace_id` / `meta.yaml.agent_tool`，后续单次调起优先从 `meta.yaml` 读取，无需调用方每次重复注入。

## 3. 工作目录结构

```
${WORKDIR}/
├── req.md                  # 原始需求 + 技术澄清章节
├── context.md              # 当前 phase 的上下文白名单
├── meta.yaml               # 需求级状态机 + 元数据（唯一契约）
├── questions.md            # 澄清问题与答复（四态状态机）
├── spec.md / plan.md / research.md / data-model.md
├── plan-report.md
├── tasks.md
├── tasks-report.md
├── validate-arch-report.md / validate-security-report.md / validate-codereview-report.md
├── commit.md               # commit 阶段产出
├── process.log             # 流式日志（串行阶段）
├── process-validate-arch.log        # validate 并行日志
├── process-validate-security.log
├── process-validate-codereview.log
├── process-validate-fix.log
└── iteration-patches/
    └── attempt-${N}.md     # 失败修补方案（外部写入，pipeline 读）
```

## 4. meta.yaml 字段定义

需求级状态文件，保存在 `${WORKDIR}/meta.yaml`。

**完整字段定义**见 `references/context-and-meta-template.md` §2。

**决策常用字段速查**：

| 你需要… | 读取字段 |
|---------|---------|
| 判断当前所处阶段 | `phase` |
| 判断是否有失败 | `last_failure`（非空 = fail 卡点） |
| 判断是否等待审查 | `pending_review`（非空 = confirm 卡点） |
| 判断重试计数 | `attempts` / `round` |
| 读取 TAPD 工作空间 ID | `workspace_id` |
| 读取代码统计 | `stats`（commit 阶段写入） |

字段语义、外部修改权限、卡点判定优先级详见
`references/state-mutation-guide.md`。

## 5. 单次执行流程

每次 pipeline 被调起，通过调用方传入的 `action` 语义指令决定行为。
pipeline 自行管理 meta.yaml 的所有状态变更——调用方不直接修改 meta.yaml。

```
1. 加载 meta.yaml（不存在 + action=execute → 按入参初始化为 phase=initialized）
2. 解析 action 指令，执行对应的状态变更：
   execute  → 首次初始化已在步骤 1 完成；非首次无变更
   approve  → phase=confirmed, pending_review=null
   reject   → 读 attempt-md 确定 target_phase, phase=target_phase, attempts+1, pending_review=null
   answer   → 校验 questions.md 无 [open], round+1
   retry    → last_failure=null, attempts+1; 若有 attempt-md 则按 target 回退 phase
   abort    → 写 last_failure.type=user_aborted, 退出
3. 启动校验（见 references/state-mutation-guide.md §7）：
   产物一致性 / questions.md 格式 / attempt-md 存在性
   不合法 → 写 last_failure.type=mutation_invalid 退出
4. 根据 phase 调用对应子 skill：
   initialized      → tapd-story-specify  → tech-clarified
   tech-clarified   → tapd-story-plan     → researched
   researched       → tapd-story-tasks    → tasks-generated
   tasks-generated  → 写 pending_review，退出（confirm 卡点）
   confirmed        → tapd-story-implement → implemented
   implemented      → tapd-story-validate  → validated
   validated        → tapd-story-commit    → committed
   committed        → 退出（完工）
5. 子 skill 返回：
   5a. 记录成本度量（每次 subagent 回传后执行）：
       1) 读取 ${WORK_DIR}/cost-events.jsonl 末尾
       2) 倒序查找匹配本次调用的事件：
          stage == 当前 stage AND attempt == 当前 attempt
          AND round == 当前 round AND ts_marker == 本次渲染 prompt 时使用的 TS
       3) 找到则 append 到 meta.yaml.stats.cost.per_call[]，
          并累加到 total_input_tokens / total_output_tokens /
          total_cache_tokens / total_cached_write_tokens / total_cached_miss_tokens /
          total_credit / total_cost_usd（与 total_credit 同值）/
          total_duration_sec，subagent_calls +1
       4) 找不到（hook 未触发 / 写入失败 / 超时未到达）：
          记录一条 warning 到 process.log，本次调用不累加，
          subagent_calls 仍 +1（保持调用次数口径准确）
       此步骤纯记录，不影响 phase 推进。
   ok       → 推进 phase，回到步骤 4 继续推进
   blocked  → 写入 questions.md（追加 [open] 条目），退出
   fail     → 写入 last_failure（按错误类型分 system / semantic），退出
6. 退出前：刷新 history，落盘 meta.yaml
```

**"推进到下一个卡点才退出"** 的实现：步骤 5 ok 后回步骤 4 继续推进，而不是退出。
唯一例外是 `tasks-generated → confirm 卡点`——因 confirm 必须由外部决策。

## 6. confirm 卡点（内联逻辑，不是独立子 skill）

confirm是本主 SKILL 的内联逻辑：

**进入条件**：phase == tasks-generated 且 `tapd-story-tasks` 返回 ok。

**行为**：

1. 把 `spec.md`、`plan.md`、`tasks.md` 路径写入 `meta.yaml.pending_review.artifacts`
2. `pending_review.ready_at` 写入当前 ISO 8601 时间戳
3. 退出 pipeline（不调用 implement）

**推进条件（下次调起时）**：调用方发送语义指令 `approve`（通过）、`reject`（回退）或 `abort`（放弃）。
pipeline 收到指令后自行执行 meta.yaml 状态变更（见 §5 步骤 2）。

## 7. 子 skill 编排

| 顺序 | 子 skill | 输入 | 输出 phase | 卡点能力 |
|------|----------|------|----------|---------|
| 1 | `tapd-story-specify` | req.md + context.md | tech-clarified | 可 blocked |
| 2 | `tapd-story-plan` | spec.md + context.md | researched | 可 fail |
| 3 | `tapd-story-tasks` | plan.md + context.md | tasks-generated | 可 fail |
| — | （内联）confirm 卡点 | spec / plan / tasks | confirmed（由外部）| 必然退出 |
| 4 | `tapd-story-implement` | tasks.md + context.md | implemented | 可 fail |
| 5 | `tapd-story-validate` | 代码变更 + context.md | validated | 可 fail |
| 6 | `tapd-story-commit` | 所有产物 | committed | 可 fail；写 stats |

每个子 skill 通过其 SKILL.md 定义详细行为。pipeline 主编排只负责
"读 meta → 选子 skill → 落地结果"，不重新编排子 skill 内部细节。

## 8. 与外部的通信契约

### 8.1 退出报告格式（主会话最后一条消息）

pipeline 退出前输出简短摘要：

```
Pipeline 已退出
- 需求 ID: ${ID}
- 工作目录: ${WORKDIR}
- 当前 phase: <phase>
- 卡点类型: <committed | confirm | blocked | fail | abort>
- 下一步指令: <approve | reject | answer | retry | abort>
- 说明: <简要描述卡点原因>
```

不要输出 `process.log` 内容，不要询问后续操作——退出后由调用方决策。

### 8.2 调用方语义指令

调用方通过语义指令与 pipeline 交互，**不直接修改 meta.yaml**。
完整指令集及触发条件见 `references/state-mutation-guide.md` §2。
调用方可修改的内容文件见 `references/state-mutation-guide.md` §5。

### 8.3 契约版本

本契约基于 `tapd-story-pipeline v1.1.0` 语义指令模式。如 pipeline 新增指令或修改卡点类型，需同步更新 `references/state-mutation-guide.md` 和 runner 调度逻辑。

## 9. Subagent 调度与日志

pipeline 内部 `specify` / `plan` / `tasks` / `implement` / `validate` 等"重 token"子 skill
通过 `Task(subagent_name="speckit-executor-agent|code-reviewer", ...)` 拉 subagent，subagent 在隔离上下文中
通过 `use_skill(command="speckit-<cmd>")` 加载对应 speckit skill 完成阶段任务；
阶段事件日志（banner / 错误 / 产物路径）追加到 `${WORKDIR}/process.log`。
成本数据由宿主 IDE PostToolUse hook 自动采集到 `${WORKDIR}/cost-events.jsonl`，
pipeline 步骤 5a 倒序匹配后写入 `meta.yaml.stats.cost`（详见 §5 步骤 5a）。

详细 prompt 模板见 `references/subagent-prompt-template.md`。
代问 / 回退重入协议见 `references/reentry-protocol.md`。
错误分类与 `last_failure` 字段格式见 `references/error-handling.md`。

> `tapd-story-commit` 是唯一在主会话中直接执行的子 skill，不拉 subagent。

## 10. 平台约定

| OS | Shell | 备注 |
|----|-------|------|
| Linux / macOS | Bash | 默认 |
| Windows | PowerShell | 首次运行设置 `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8` |

## 11. 参考文件

| 文件 | 用途 |
|------|------|
| `references/state-mutation-guide.md` | **必读**：语义指令集 + 状态管理规则 + 内容文件权限 |
| `references/context-and-meta-template.md` | meta.yaml / context.md 模板与字段说明 |
| `references/subagent-prompt-template.md` | 子 skill 内调用 speckit 的 SUBAGENT_PROMPT 骨架 |
| `references/reentry-protocol.md` | 代问重入 / 回退重入协议 |
| `references/shared-reentry-conventions.md` | 子 skill 通用可重入约定 |
| `references/questions-md-template.md` | questions.md 四态状态机 |
| `references/error-handling.md` | 需求层错误处理规则 |
| `references/commit-conventions.md` | commit 阶段规范 |
| `tapd-story-specify/references/technical-clarification-guide.md` | 技术澄清维度与最佳实践 |
| `tapd-story-specify/references/technical-clarification-template.md` | 技术澄清文档模板 |

## 12. Example: 单需求 Happy Path

用户输入："帮我实现需求 #1234567890，workspace 20000001"

**第 1 次调起（initialized → tasks-generated）：**
- 加载 meta.yaml（不存在 → 初始化 phase=initialized）
- specify：技术澄清 + subagent 生成 spec.md → phase=tech-clarified
- plan：subagent 生成 plan.md + plan-report.md（verdict=pass）→ phase=researched
- tasks：subagent 生成 tasks.md + tasks-report.md（verdict=pass）→ phase=tasks-generated
- 写入 pending_review → **退出（confirm 卡点）**

**外部操作：**
- 用户审查 spec.md / plan.md / tasks.md → 通过
- 设置 phase=confirmed, pending_review=null

**第 2 次调起（confirmed → committed）：**
- implement：subagent TDD 实现 + 测试全绿 → phase=implemented
- validate：三段并行校验全 LGTM → phase=validated
- commit：统计变更 + 构建 commit message + git add/commit + TAPD 状态更新 → phase=committed
- **退出（完工）**
