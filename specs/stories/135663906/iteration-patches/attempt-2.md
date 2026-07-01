# Attempt 2 — Improvement Notes

## 失败阶段
failed_phase: confirm（评审卡点深挖，非执行失败）

## 回退目标
target_phase_to_re_enter: initialized（重做 specify → plan → tasks；技术澄清结论已写入 req.md「技术澄清补充（第 2 轮 / attempt-2）」+ questions.md Q-008~Q-011，重跑时无需再澄清这些点）

## 根因
root_cause: |
  首轮 spec/plan/data-model 对「期望托管态来源」「比对字段集合」「.proc 真实格式」表述过粗或与事实不符：
  1. 比对字段把 procName 当作 ProcessInfo 字段（实际 procName 来源是 Process.Spec.FuncName，ProcessInfo 无此字段）。
  2. 未处理「bscp 不下发 versionCmd/healthCmd 而 .proc 含这两字段」——若纳入「期望项 ⊆ 实际项」子集比对会恒误判。
  3. 「已托管/未托管」判定基准未落到具体字段；经核实 bscp 无 is_auto 等价字段，应以 ProcessInstanceSpec.ManagedStatus 为基准。
  4. .proc 真实命名（驼峰）与多来源 contact 过滤（GSEKIT_BIZ_ vs nodeman）未在产物中确认。

## 代码保留策略
code_preserved: n/a（尚未进入 implement，无代码产出）

## 期望改进点（下游重跑时优先覆盖）
expected_improvements:
  - "spec.md FR：以 ManagedStatus（managed→应托管 / unmanaged|空→不应托管 / starting|stopping→本轮跳过 / partly_managed→实例上不出现可忽略）为『是否应托管』判定基准，替换模糊的『已托管/未托管』。映射见 req.md 决策 1 与 questions.md Q-008。"
  - "spec.md / data-model.md 比对字段裁剪为 9 字段：procName(来源 Process.Spec.FuncName) / setupPath(WorkPath) / pidPath(PidFile) / user(User) / startCmd / stopCmd / restartCmd / reloadCmd / killCmd(FaceStopCmd)；显式剔除 versionCmd/healthCmd 及 GSE 内部字段(type/cpulmt/memlmt/password/userPwd/startCheck*/opTimeOut/operateType/timestamp)。见 Q-010。"
  - "data-model.md：明确 procName 来源是 Process.Spec.FuncName（非 ProcessInfo）。"
  - "data-model.md / spec.md：.proc 为驼峰命名 JSON({\"proc\":[{...}]})，解析后必须按 contact==GSEKIT_BIZ_{bizID} 过滤本业务托管项；valuekey=GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}(用别名 alias)；host 级 expected_keys=该 host 全部 bscp 实例 valuekey，illegal=actual-expected→ILLEGAL_VALUE_KEY。见 Q-009/Q-011。"
  - "比对算法对标 gsekit check_process.py：期望项(9 字段) ⊆ 实际项 即一致；差异字段名集合写入 error_msg。"
  - "测试数据：采用 samples/proc-example.json 作为 .proc 解析/比对单测基准（含 GSEKIT_BIZ_ 与 nodeman 混合项，用于验证 contact 过滤与非法 valuekey 判定）。"

## attempt-1 旧产物位置
archived_to: iteration-patches/attempt-1/（spec.md / plan.md / research.md / data-model.md / tasks.md / plan-report.md / tasks-report.md）
说明: 这批产物内容已过时（未含本文件的改进点），仅作参考/对照。重跑时以 req.md「技术澄清补充（第 2 轮）」+ questions.md Q-008~Q-011 为准，新产物直接生成到 specs/stories/135663906/ 根目录。

## 不变项（沿用 attempt-1 结论）
unchanged:
  - "crontab 接入 + IsMaster 守卫 + 跨租户按业务遍历 + GSE 脚本执行构建块复用（不引入 istep 流水线）+ rateLimiter 限流。"
  - "异常落库/恢复闭环复用上游 DAO（Create/GetLatestByProcessInstanceID/IsException/UpdateStatus）。"
  - "新增 internal/processor/processcheck 包 + cmd/data-service/service/crontab 定时任务 + pkg/cc 配置；不改表。"
