---
name: tapd-story-validate
slug: tapd-story-validate
version: 3.0.0
description: |
  迭代执行流水线校验阶段。对 implement 产出的代码做三维度并行校验（架构 / 安全 / CodeReview），
  再按汇总结果原地修复。每个维度通过独立 subagent 执行，日志写入独立 process-validate-*.log。
  命令与报告模板详见 ../references/subagent-prompt-template.md §2.6~§2.9。
  回退默认保留代码（携带 unresolved_findings 回上游修订规范）。
---

# 需求实现校验

## 前置条件

- 当前需求 phase 为 `implemented`
- 代码实现已完成（迭代分支上），**本子 skill 不创建分支、不切分支**
- pipeline 主编排已按 `references/context-and-meta-template.md` 重写当前阶段的 `context.md`
  （白名单含：spec.md / plan.md / tasks.md + 项目宪章 + 架构/安全/编码规范文档 +
  `Code scope` + 可复用 skill 入口 `skills/bk-security-redlines/`；
  CodeReview 维度由 `code-reviewer` agent 内置清单驱动，不再依赖 `skills/code-review/`）
- **本子 skill 不维护代码回滚锚点**：回退默认保留代码

## 输入

- `${WORKDIR}/meta.yaml`（读取 `attempts`）
- 当前需求 ID（由 pipeline 主编排传入）
- `${WORKDIR}/spec.md` / `plan.md` / `tasks.md`
- `${WORKDIR}/context.md`
- `${WORKDIR}/iteration-patches/attempt-${N}.md`（若为回退重入，必读）

## 执行流程

本阶段由**三段并行校验 subagent** + **一段原地修复 subagent**（仅当需要）组成。

```
                  ┌── subagent #1 ── validate-arch-report.md
attempt ${N}      │
round ${R}  ──────┼── subagent #2 ── validate-security-report.md   ── 汇总 ── subagent #4 (修复) ── round +1
                  │
                  └── subagent #3 ── validate-codereview-report.md
```

### 1. 三段并行校验（subagent #1 / #2 / #3）

pipeline 主编排按 `../references/subagent-prompt-template.md` 中的三份模板分别渲染，
**同一轮次内**三次 `Task()` 并行发起：

| subagent | 执行 agent | template 段 | 产出 | 日志 |
|----------|-----------|------------|------|------|
| #1 架构 | `speckit-executor-agent` | §2.6 validate-arch | `validate-arch-report.md` | `process-validate-arch.log` |
| #2 安全 | `speckit-executor-agent` | §2.7 validate-security | `validate-security-report.md` | `process-validate-security.log` |
| #3 CodeReview | `code-reviewer` | §2.8 validate-codereview | `validate-codereview-report.md` | `process-validate-codereview.log` |

渲染填充：`${ID}` / `${VERSION}` / `${WORK_DIR}` / `attempts` / `round`；
若 `attempts > 1`，附上 `iteration-patches/attempt-${attempts}.md` 摘要。

> 三段的命令、统一报告模板、Verdict 判定规则均在 subagent-prompt-template.md
> §2.6~§2.8 中定义，本子 skill 不重复。

### 2. 汇总决策（主会话）

| 汇总条件 | 主会话动作 |
|---------|-----------|
| 三份全 `LGTM`，且无 CRITICAL/HIGH finding | 跳过修复，推进 phase |
| 任一份 `needs_fix`，但全部归因 `code-self` | 启动原地修复（同 attempt 内 round +1）|
| 任一份 `needs_fix`，且任一归因 `spec-insufficient` / `plan-insufficient` | 走回退重入（§5） |
| 任一段 subagent `status=fail` | 按 `error-handling.md` 处理 |

### 3. 原地修复（subagent #4，仅当需要）

pipeline 主编排按 `../references/subagent-prompt-template.md` §2.9 **validate-fix 模板**渲染，
拉起修复 subagent。日志写入 `process-validate-fix.log`。

> 修复命令、自检步骤、回传 JSON 要求均在 §2.9 中定义。

修复完成后，pipeline 主编排**重新回到步骤 1** 并行跑三段校验（round +1），
直到全部 LGTM 或升级为回退重入。同 attempt 内最多 3 个 round。

> round 号在"修复 + 三段校验"整组完成后才 +1。

### 4. 更新状态

三份 verdict 均 `LGTM` 时：phase → `validated`；history 追加成功记录。

### 5. 信息不足回退（回退重入入口，**保留代码**）

**回退触发条件**：

| 触发条件 | 动作 |
|---------|------|
| `needs_fix` 且任一归因 `spec-insufficient` / `plan-insufficient` | 走回退重入 |
| 同 attempt 内 round 重试 3 次仍有 CRITICAL/HIGH finding | 强制升级为回退重入 |

**回退动作清单**（validate 阶段特化）：

| 步骤 | 动作 | 备注 |
|------|------|------|
| 1 | 三份评审报告原样保留 | 作为上游修订的参考证据 |
| 2 | 写 `iteration-patches/attempt-${N+1}.md` | 固定 `code_preserved: true`；必填字段见 state-mutation-guide §2 |
| 3 | 更新 meta.yaml | `attempts` +1；`last_failure` 写入摘要；`history` 追加 `code_preserved_rollback` |
| 4 | 不做 git 回滚 | 代码、评审报告、日志全部保留 |
| 5 | 跳转到 `target_phase_to_re_enter` | 下游按差量修复模式工作 |

**下游复用代码行为**：

| 子 skill | 行为 |
|---------|------|
| implement | 不重建 baseline_commit；注入 unresolved_findings，做差量修复 |
| validate | 重走三段并行校验；新 attempt rounds 从 1 开始 |

## 可重入约定

两类重入，**都不主动回滚代码**：

- **同 attempt round 重试 = 原地修复**：评审报告覆盖重写，历史经 process-validate-*.log 回溯
- **回退重入 = 保留代码 + 携带问题回上游**（详见 §5）

| 重入类型 | pipeline 主编排动作 | meta.yaml 变化 |
|---------|-----------|----------|
| round 重试 | 不动代码；覆盖重写评审报告；拉修复 subagent | round +1，attempts 不变 |
| 回退重入 | 写 `last_failure`；调用方写 `attempt-*.md` + 改 phase | attempts +1、phase 切换 |

**严禁**：`git reset --hard` / `git stash` / `git checkout <branch>` / `git clean -fdx`。

子 skill 在执行前必须：

- 读取最新 `attempt-*.md`（若有），追加重入增量段（subagent-prompt-template.md §3）
- 若 `code_preserved=true`，在 SUBAGENT_PROMPT 中告知"重新评审当前代码与新规范的对齐"
- 校验 `Code scope` 白名单未被意外缩窄

幂等性、各 process-validate-*.log 仅追加等通用约束见 `../references/reentry-protocol.md`。

## 参考资料

- SUBAGENT_PROMPT（validate-arch §2.6 / validate-security §2.7 / validate-codereview §2.8 /
  validate-fix §2.9）+ 统一报告模板：`../references/subagent-prompt-template.md`
- 通用回传 JSON Schema：`../references/subagent-prompt-template.md` §1.3
- 上下文与元数据模板：`../references/context-and-meta-template.md`
- 可重入协议：`../references/reentry-protocol.md`
- 错误处理：`../references/error-handling.md`
- 可复用 skill：`skills/bk-security-redlines/`（安全维度）
- CodeReview 维度执行 agent：`agents/code-reviewer.md`（内置分级评审清单）

## 产出

- `${WORKDIR}/validate-arch-report.md` — 架构校验报告
- `${WORKDIR}/validate-security-report.md` — 安全校验报告
- `${WORKDIR}/validate-codereview-report.md` — CodeReview 报告
- `${WORKDIR}/process-validate-arch.log` — 架构校验日志
- `${WORKDIR}/process-validate-security.log` — 安全校验日志
- `${WORKDIR}/process-validate-codereview.log` — CodeReview 日志
- `${WORKDIR}/process-validate-fix.log` — 修复日志（仅当修复执行时）
- `${WORKDIR}/meta.yaml` — 更新后的 attempts / round / validate_attempts / history
- 需求 phase 为 `validated`
