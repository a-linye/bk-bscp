# validate-arch Report — Story 1020451610135663906

## Verdict
LGTM

## Checked artifacts
- internal/processor/processcheck/parse.go
- internal/processor/processcheck/expected.go
- internal/processor/processcheck/compare.go
- internal/processor/processcheck/record.go
- internal/processor/processcheck/executor.go
- internal/processor/processcheck/checker.go
- cmd/data-service/service/crontab/check_managed_process.go
- cmd/data-service/app/app.go（startCronTasks 守卫启动）
- pkg/cc/types.go（CheckProcessManagedConfig + CrontabConfig 串接）
- cmd/data-service/etc/data_service.yaml（样例）

## Reference baselines
- specs/stories/135663906/plan.md（文件级改动 + 复用约定 + 约束自检）
- specs/stories/135663906/research.md（D1 复用脚本执行不复用 istep / D9 限流 / D11 可测性 seam）
- specs/stories/135663906/context.md（Code scope 白名单）
- AGENTS.md（不引入不必要抽象 / 复用优先 / 纯新增向后兼容）

## Findings

### A1
- **类别**：Architecture
- **严重性**：MEDIUM
- **位置**：internal/processor/processcheck/executor.go:44-54
- **总结**：`NewGSEScriptRunner` 在核心逻辑包内直接读取全局单例 `cc.G().GSE` / `cc.G().TaskFramework`，与其它配置（interval/script/qpsLimit 由参数注入）的注入风格不一致，全局状态内嵌降低边界清晰度与可测性。
- **根因**：code-self
- **修改建议**：可接受现状——这是对既有 `common.Executor`（其 `GseConf`/`TaskConf` 亦取自 `cc.G()`）模式的等价复用，且 `ScriptRunner` 接口已把测试边界隔离干净（checker 单测注入 fake runner，不触达全局配置）。若后续需要更强可测性，可将 `GSE`/`TaskFramework` 配置作为参数传入 `NewGSEScriptRunner`。非阻断。

### A2
- **类别**：Architecture
- **严重性**：MEDIUM
- **位置**：internal/processor/processcheck/executor.go:22-23
- **总结**：`processcheck`（processor 层）反向引用 `internal/task/executor/common` 与 `internal/task/executor/config`，而 `task/executor` 既有包（如 `task/executor/process`、`config/config_generate.go`）又依赖 `internal/processor`，两层在包粒度上形成双向耦合。
- **根因**：code-self
- **修改建议**：可接受——经 `go build ./internal/processor/processcheck/... ./cmd/data-service/...` 验证无 import 循环（依赖落在 `processcheck → task/executor/{common,config}`，与 `task/executor/process → processor/gse` 是不同包，不构成环）。该复用是 plan/research D1 明确认可的「复用 GSE 异步脚本执行 + 轮询构建块、不重写、不引入 istep」的直接落地，符合复用优先约束。建议仅在文档/注释保留边界说明，避免后续在 `common`/`config` 中反向引入 `processcheck` 造成真实环。非阻断。

### A3
- **类别**：Architecture
- **严重性**：LOW
- **位置**：cmd/data-service/service/crontab/check_managed_process.go:34-58 / cmd/data-service/app/app.go:601-606
- **总结**：定时任务入口与 `startCronTasks()` 守卫完整对齐既有 crontab 样板（ticker + `shutdown.AddNotifier()` + `IsMaster()` 守卫 + 默认 `Enabled=false` 纯新增），分层、注入（daoSet/sd/gseSvc）方向均正确。
- **根因**：code-self
- **修改建议**：无需修改，记录为符合项。

## 架构维度结论汇总

1. **分层架构 / 依赖方向**：`processcheck` 依赖方向整体正确——向下依赖 `internal/components/gse`（命名空间/类型）、`pkg/dal/table`、`internal/dal/dao`（数据访问）；同层复用 `internal/processor/gse.BuildProcessOperate`。无 dao/components 反向依赖 `processcheck` 的情况。横向复用 `task/executor` 见 A2（已评估，无环、属认可复用）。
2. **循环依赖**：`go build` 主模块（processcheck + cmd/data-service）与 `pkg/cc` 模块均通过（EXIT=0），无 import 循环。`processcheck` 仅被 `crontab/check_managed_process.go` 引用，无反向引用。
3. **模块边界**：复用 `BuildProcessOperate`（含 ProcName=FuncName 渲染）、`common.Executor.WaitExecuteScriptFinish`、上游 `dao.ProcessManagedException`，未重造；`executor.go` 仅取 `common.Executor` 执行能力，**未**引入 `config_check.go` 的 istep step/callback 流水线，符合 research D1。`ScriptRunner` 为必要测试 seam。
4. **Code scope 白名单**：`git status --short` 改动文件（app.go / data_service.yaml / types.go / check_managed_process.go / processcheck/**）全部落在 context.md `## Code scope` 内。`pkg/cc/service.go` 列于 scope 但未改动（默认/校验在 types.go 完成，符合「若在此则补串接」的条件式约定）。`bk-process-config-manager/` 为 gsekit 参考源码目录（白名单引用其文件用于对标），非本需求代码产物，不计入 Code scope 违规。

> 无 CRITICAL/HIGH（[必须]）级架构问题；A1/A2 为 [建议]、A3 为 [Nit]，均不阻断。
