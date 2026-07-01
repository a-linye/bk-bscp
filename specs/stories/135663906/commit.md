# Commit 记录

## Commit Message

```
feat(processcheck): 新增进程托管配置定时检查与异常闭环

新增 data-service 定时巡检任务，按业务跨租户逐个扫描进程实例，比对 bscp
期望托管配置与 GSE 实际 .proc 托管配置的一致性，识别异常写入上游「托管
异常记录」，并在后续检查恢复一致时自动解除异常态，形成检查—记录—恢复闭环。

- 新增 internal/processor/processcheck 核心包：.proc 驼峰 JSON 解析（按
  contact==GSEKIT_BIZ_{bizID} 过滤本业务项）、9 字段子集比对（procName 来源
  Process.Spec.FuncName）、host 级非法 valuekey 判定、以 ManagedStatus 为
  应托管基准的逐实例分类、异常落库与恢复闭环决策，均下沉为可单包单测的纯函数。
- 新增 cmd/data-service/service/crontab/check_managed_process.go 定时任务
  入口（ticker + shutdown + IsMaster 守卫 + 跨业务遍历 + rateLimiter 限流）。
- cmd/data-service/app/app.go 按 Enabled 守卫启动新巡检任务。
- pkg/cc 新增 CheckProcessManagedConfig（Enabled/Interval/QpsLimit + .proc
  脚本配置）与默认值/校验，data_service.yaml 增样例（默认 enabled:false）。
- 复用上游 #135663687 的托管异常记录 DAO，不新增表/字段。

--story=1020451610135663906
```

## Commit Hash

<见提交后回填>

## 变更统计

| 指标 | 值 |
|------|-----|
| 总变更行数 | 4424 |
| 新增代码 | 4424 |
| 删除代码 | 0 |
| 逻辑代码 | 954 |
| 测试代码 | 693 |
| 文档变更 | 2446 |
| 变更文件数 | 41 |

> 已排除无关目录 `bk-process-config-manager/`（gsekit 参考代码，非本需求实现）。

## 成本汇总

### 总体

| 指标 | 值 |
|------|-----|
| 总耗时 | 0 s（未采集）|
| 总成本 | 0 credit（未采集）|
| 总输入 tokens | 0（未采集）|
| 总输出 tokens | 0（未采集）|
| 总缓存 tokens | 0（未采集）|
| subagent 调用次数 | 13（attempt-1=5 + attempt-2=8）|

> 本环境无 `cost-events.jsonl`，宿主 IDE PostToolUse hook 未采集成本数据；成本总量保持 0，仅记录 subagent 调用次数。

### 各阶段

| 阶段 | 耗时 | 成本 | 输入 tokens | 输出 tokens | 缓存 tokens | 调用次数 |
|------|------|------|------------|------------|------------|---------|
| —（未采集）| — | — | — | — | — | — |

## 时间

- 开始时间：2026-06-30T17:56:00+08:00
- 完成时间：2026-06-30T20:42:00+08:00
