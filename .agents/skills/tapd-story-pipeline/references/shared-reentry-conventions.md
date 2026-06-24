# 子 Skill 通用可重入约定

> 本文档定义 plan / tasks / implement / validate 四个子 skill 共享的可重入规则。
> 各子 skill 的 SKILL.md 引用本文档，仅补充差异项。
> specify 的可重入约定有所不同（代问重入），不在此文档范围内。

## 两类重入

1. **回退重入**（外部触发）：调用方按 `state-mutation-guide.md` §2 卡点 3
   写入 `iteration-patches/attempt-${N}.md`、`meta.yaml.attempts` +1
2. **同 attempt 内 round 重试**（pipeline 内部）：`meta.yaml.round` +1（attempts 不变）
3. **每次调用前**：pipeline 主编排重新生成 `context.md`（合并 `patch_to_context`），
   `context_revision` +1

## 子 skill 启动前必做

- 读取最新 `attempt-*.md`（若有），追加重入增量段（`subagent-prompt-template.md` §3）
- 校验 `Code scope` 白名单未被意外缩窄（仅 implement / validate 阶段）

## 通用约束

- **幂等性**：重复执行不产生副作用
- **产物保留**：不手工删除前一轮产物，由 subagent 覆盖重写
- **process.log 仅追加**：不截断、不清空
- 详见 `reentry-protocol.md`

## 各阶段差异速查

| 阶段 | round 重试触发 | 报告文件处理 | 特殊约束 |
|------|--------------|-------------|---------|
| plan | `verdict=needs_fix` | `plan-report.md` 覆盖重写 | — |
| tasks | `verdict=needs_fix` | `tasks-report.md` 覆盖重写 | 两段 subagent 均追加增量段 |
| implement | test fail → 原地修复 | 无报告文件 | 不回滚代码；`implement_attempts` 追加 issue |
| validate | 修复+重校验整组完成后 +1 | 三份评审报告覆盖重写 | 严禁 git reset；`code_preserved` 信号 |
