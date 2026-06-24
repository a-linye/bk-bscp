---
name: tapd-story-plan
slug: tapd-story-plan
version: 3.0.0
description: |
  迭代执行流水线开发计划阶段。基于 spec.md 调用 /speckit.plan 以测试驱动开发模式构建
  开发计划、技术调研与数据模型，并在同一 subagent 内就地做文档级合规自检，产出 plan-report.md。
  speckit 命令与报告模板详见 ../references/subagent-prompt-template.md §2.2。
---

# 需求实现计划

## 前置条件

- 当前需求 phase 为 `tech-clarified`
- `${WORKDIR}/spec.md` 已存在
- pipeline 主编排已按 `references/context-and-meta-template.md` 重写当前阶段的 `context.md`
  （白名单包含：spec.md + 项目宪章 + 架构/安全/编码规范引用文档）

## 输入

- `${WORKDIR}/meta.yaml`（读取 `attempts`）
- 当前需求 ID（由 pipeline 主编排传入）
- `${WORKDIR}/spec.md`
- `${WORKDIR}/context.md`
- `${WORKDIR}/iteration-patches/attempt-${N}.md`（若为回退重入，必读）

## 执行流程

pipeline 主编排按 `../references/subagent-prompt-template.md` §2.2 **Plan 模板**渲染
SUBAGENT_PROMPT（含 speckit.plan skill + 就地合规自检 + plan-report.md 产出），
通过 `Task(subagent_name="speckit-executor-agent", ...)` 拉起 subagent。

渲染填充：`${ID}` / `${VERSION}` / `${WORK_DIR}` / `attempts` / `round`；
若 `attempts > 1`，附上 `iteration-patches/attempt-${attempts}.md` 摘要。

> subagent 内的两段工作（speckit-plan + 就地合规自检）、合规自检 checklist（3 个维度：
> 完整度 / research 合规 / 项目宪章）、plan-report.md 统一模板、Verdict 判定规则
> 均在 subagent-prompt-template.md §2.2 中定义，本子 skill 不重复。

### 消费 subagent 回传 JSON

按 `compliance.verdict` 分支：

| Verdict | 主会话动作 |
|---------|-----------|
| `pass` | 进入"更新状态"推进 phase |
| `needs_fix` | 同 attempt 内 round +1，重跑 plan（最多 3 个 round） |
| `spec_insufficient` | 整理 findings 摘要为 `iteration-patches/attempt-${N+1}.md`，`target_phase_to_re_enter=specify`，走回退重入 |

`status=fail` 按 `references/error-handling.md` 处理，一般升级为回退重入。

### 更新状态

`compliance.verdict=pass` 时：

- 更新 `meta.yaml.phase` 为 `researched`
- `meta.yaml.history` 追加成功记录，清空 `meta.yaml.last_failure`

## 可重入约定

通用规则见 `../references/shared-reentry-conventions.md`。本阶段差异：

- **round 重试触发条件**：`compliance.verdict=needs_fix`，同 attempt 内最多 3 个 round
- **报告文件不做快照**：`plan-report.md` 每轮覆盖重写，历史可在 process.log 中追溯
- 读取最新 `attempt-*.md`（若有），在 SUBAGENT_PROMPT 中追加重入增量段到 `/speckit.plan` 提示词中
  （subagent-prompt-template.md §2.2）

幂等性、产物保留、process.log 仅追加等通用约束见 `../references/reentry-protocol.md`。

## 信息不足回退（回退重入入口）

当 `verdict=spec_insufficient` 或下游子 skill 反向追溯到本阶段信息不足时，
`attempt-${N+1}.md` 必须明确：

- `failed_phase: plan`
- `root_cause`：可直接复用 `plan-report.md` 中对应 Finding 的根因字段
- `target_phase_to_re_enter: specify`（spec 不足）或 `plan`（仅 plan 自身）

## 参考资料

- SUBAGENT_PROMPT（plan 模板 + 合规自检 checklist + 报告模板）：`../references/subagent-prompt-template.md` §2.2
- 通用回传 JSON Schema：`../references/subagent-prompt-template.md` §1.3
- 上下文与元数据模板：`../references/context-and-meta-template.md`
- 可重入协议：`../references/reentry-protocol.md`
- 错误处理：`../references/error-handling.md`

## 产出

- `${WORKDIR}/plan.md` — 开发计划
- `${WORKDIR}/research.md` — 技术调研报告
- `${WORKDIR}/data-model.md` — 数据模型（如有）
- `${WORKDIR}/contracts/*.md` — 通讯协议（如有）
- `${WORKDIR}/plan-report.md` — 文档级合规自检报告
- `${WORKDIR}/process.log` — stream-json 流式日志（追加）
- `${WORKDIR}/meta.yaml` — 更新后的 attempts / round / history
- 需求 phase 为 `researched`
