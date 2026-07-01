# Validate-Security Report — Story 1020451610135663906

## Verdict
LGTM

## Checked artifacts
- internal/processor/processcheck/parse.go
- internal/processor/processcheck/compare.go
- internal/processor/processcheck/expected.go
- internal/processor/processcheck/executor.go
- internal/processor/processcheck/checker.go
- internal/processor/processcheck/record.go
- cmd/data-service/service/crontab/check_managed_process.go
- cmd/data-service/app/app.go（startCronTasks 守卫启动段，L601-606）
- pkg/cc/types.go（CheckProcessManagedConfig + validate/trySetDefault，L1248-1464）
- cmd/data-service/etc/data_service.yaml（crontab.checkProcessManaged 样例，L197-208）

## Reference baselines
- .cursor/skills/bk-security-redlines/SKILL.md（三大红线：输入校验 / 鉴权 / 敏感数据）
- specs/stories/135663906/context.md（Code scope 白名单 + 安全维度）
- AGENTS.md（仓库协作/Go 规范）

## Findings

> 校验维度：1) 输入校验  2) 鉴权（跨租户隔离/越权）  3) 敏感数据  4) 常见风险（命令注入/路径穿越/不安全反序列化/SSRF）。
> 下述结论均针对本次新增/改动代码（Code scope）。

### 红线核对结论

- **红线 1（外部输入未校验）**：未发现违规。外部输入来源为 GSE agent 的 `.proc` Screen 文本，
  进入 `ParseProcScreen` 后仅做「regex 抽取首个 JSON 对象 → `json.Unmarshal` 到定长 typed struct（`procEnvelope`/`ActualProc`）→ 字符串比对」。
  解析出的 `startCmd/stopCmd/...` 等字段**仅用于与期望值做等值比对（`diffFields`），从不被本服务执行或拼接进任何命令/路径/SQL**，
  因此不存在「外部输入进入高危操作」的链路。抽取正则 `(?s)\{.*\}` 为线性匹配，无灾难性回溯（ReDoS）风险；解析失败统一收敛为 `ErrParsing`/`ErrAgentException`。
- **红线 2（敏感接口未鉴权）**：未发现违规。本需求新增的是 data-service **内部定时任务**（非对外接口），
  启动受 `crontabConfig.CheckProcessManaged.Enabled` 开关 + 运行期 `state.IsMaster()` 双守卫（slave 不下发任何脚本，`check_managed_process.go` L79-83）。
  未新增任何对外 HTTP/RPC 端点。
- **红线 3（敏感数据未加密）**：未发现违规。无硬编码凭证/密钥；脚本执行账户经 `config.GetExecutionUser(fileMode, "")` 取默认托管账户（非明文密码）；
  `error_msg`/日志均不落入 `.proc` 原始 Screen 内容与命令明文（详见 A2）。

### A1
- **类别**：Security（鉴权 / 跨租户隔离）
- **严重性**：LOW
- **位置**：cmd/data-service/service/crontab/check_managed_process.go:91-110；internal/processor/processcheck/parse.go:75-83
- **总结**：跨业务/跨租户遍历的租户隔离实现正确，无横向越权。
- **根因**：code-self（无缺陷，记录核对结论）
- **修改建议**：无需修改。核对要点：`checkAllBiz` 按 `ListBizTenantMap` 逐 biz 设置 `bizKit.TenantID = tenantID` 并 `bizKit.Ctx = bizKit.InternalRpcCtx()`，
  与白名单样板 `sync_cmdb.go` 的按租户遍历模式一致，DB 取数（`ListProcessesWithInstance`/`GetByProcessIDs`）随 kit 携带租户上下文；
  且 `ParseProcScreen` 以 `contact == GSEKIT_BIZ_{bizID}` 过滤同一 `.proc` 内的他业务托管项，比对前即剔除跨业务数据，避免一台主机多业务托管导致的越权读判定。
  并发 goroutine（`runChecks`）共享同一 biz 的 `bizKit`，同业务同租户，无租户串话。

### A2
- **类别**：Security（敏感数据 / 日志泄露）
- **严重性**：LOW
- **位置**：internal/processor/processcheck/compare.go:107,134,138,147；checker.go:156-169；record.go:38-44
- **总结**：异常记录与日志不泄露 `.proc` 原始内容与命令明文。
- **根因**：code-self（无缺陷，记录核对结论）
- **修改建议**：无需修改。`error_msg` 仅含 valuekey 与差异**字段名**（如 `差异字段: [startCmd, user]`）、固定文案与 `ErrParsing/ErrAgentException` 的固定 error 串，
  不含 `startCmd/user/pidPath` 等字段的**取值**；`logs.Errorf` 仅打印 bizID/agentID/instID/err，不打印 Screen 全文。落库的 `ProcessManagedExceptionSpec` 只含 error_type/error_msg/suggestion/status/checked_at，无命令明文持久化。

### A3
- **类别**：Security（命令注入 / 配置可信边界）
- **严重性**：LOW
- **位置**：internal/processor/processcheck/executor.go:59-100；pkg/cc/types.go:1447-1453；data_service.yaml:205-208
- **总结**：下发脚本内容来自服务端管理员配置（`linuxProcScript`/`windowsProcScript`），非外部/租户输入，缺省为只读 `cat`/`type .proc`；不构成命令注入红线。
- **根因**：code-self（设计取舍，非缺陷）
- **修改建议**：无需代码修改。提示运维：该配置项为「直接下发到 agent 执行的 shell 命令」，应通过受控配置渠道维护并限定为只读读取 `.proc`，避免被改写为任意命令在全量 agent 上执行。
  注：`scriptName` 用 `time.Now().UnixNano()` 生成、`osType` 仅在 linux/windows 两个固定分支间选择、`agentID` 作为 GSE API 结构化字段（`BkAgentID`）传递而非 shell 插值，均无注入面。

### A4
- **类别**：Security（不安全反序列化 / 路径穿越 / SSRF）
- **严重性**：LOW
- **位置**：internal/processor/processcheck/parse.go:70-73；expected.go:58-62
- **总结**：未发现不安全反序列化、路径穿越、SSRF。
- **根因**：code-self（无缺陷，记录核对结论）
- **修改建议**：无需修改。两处 `json.Unmarshal` 均反序列化到定长 typed struct（`procEnvelope`、`table.ProcessInfo`），Go 标准库 JSON 不触发代码执行；
  失败即 `return nil`/`ErrParsing` 并跳过，具备健壮性。无外部输入参与文件路径拼接；GSE 调用目标（endpoint/agentID）来自配置与可信 DB，不存在 SSRF。

## 备注（验收对照）
- 无 [必须]（CRITICAL/HIGH）项；A1~A4 均为 LOW 级核对结论或运维提示。
- 依据 report-template.md「validate 三段」判定规则：无 [必须] 项 → Verdict=LGTM。
