# 任务清单：进程托管配置定时检查与异常闭环

**需求 ID**：短 ID 135663906 / 长 ID 1020451610135663906
**输入**：`plan.md`（Phase 2 TDD 八步 + 文件级改动）、`research.md`（D1~D12）、`data-model.md`（§2 运行态结构 + §3 比对/分类规则）、`spec.md`（FR-001~FR-015、SC-001~007、AC-001~004/AC-P01/AC-T01~T04、US1 P0 / US2 P1）
**模式**：测试驱动开发（TDD）——纯逻辑（parse / compare / record 决策）先写测试再实现；编排级用 fake `ScriptRunner` + fake DAO 集成
**上下文白名单**：见 `context.md`（代码改动仅触达 Code scope 路径）

## 说明

- 本子需求为**核心检查引擎**（编排 + 比对），复用上游 #135663687 的存储能力，不新增表/字段，纯新增配置项向后兼容（默认 `enabled: false`）。
- **attempt-2 校正基线**：以 `ProcessInstanceSpec.ManagedStatus` 为「是否应托管」基准；比对字段裁剪为 **9 字段**（`procName` 来源 `Process.Spec.FuncName`，其余 8 字段来源 `ProcessInfo` 渲染）；`.proc` 为驼峰 JSON，解析后必须按 `contact==GSEKIT_BIZ_{bizID}` 过滤；valuekey=`GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`（别名 alias）；host 级 `illegal_keys=actual-expected`→`ILLEGAL_VALUE_KEY`；单测以 `samples/proc-example.json` 为基准。
- 运行态结构体不单设 `types.go`，按 `plan.md` 文件清单就近定义：`ActualProc` 在 `parse.go`、`ExpectedProc` 在 `expected.go`、`CheckResult` 在 `compare.go`（避免引入冗余文件，依据 AGENTS.md「不引入不必要抽象」）。
- **测试取舍（research D11 / plan §Phase2.9）**：`.proc` 解析（含 contact 过滤）、逐项比对+分类（ManagedStatus 分支 + 9 字段子集 + illegal）、写异常/恢复决策为纯函数/接口 fake 可验证，作 TDD 强约束；跨租户遍历 + 真实 GSE 下发 + 真实 DB 写入的端到端路径依赖带 DB/GSE 环境，以集成 mock（fake `ScriptRunner` + fake DAO）+ 代码评审保障，不引入 sqlite/sqlmock。
- `record.go` 的写入/恢复决策在 US1（异常→`Create`）落地，US2（通过且 `IsException`→`GetLatestByProcessInstanceID`+`UpdateStatus(recovered)`）增量补齐；`checker.go` 在 US1 调用决策函数，恢复分支随 US2 完整化。
- `[P]` 标记表示可与同 Phase 内其他 `[P]` 任务并行（不同文件、无未完成依赖）；测试文件与被测实现文件不同名，故测试任务多可 `[P]`。

---

## Phase 2：基础链路（Foundational — 阻塞所有用户故事，必须先完成）

- [ ] T001 [FR-004/FR-013/FR-014] 在 `pkg/cc/types.go` 新增 `CheckProcessManagedConfig{ Enabled bool; Interval string; QpsLimit float64; LinuxProcScript string; WindowsProcScript string }`（yaml key `checkProcessManaged`，字段见 data-model §4），挂到 `CrontabConfig`；实现其 `trySetDefault()`（Interval 空→`20m`、QpsLimit 0→`80.0`、两个脚本字段空→对标 gsekit 缺省 `cat /usr/local/gse2_bkte/agent/etc/.proc` / `type c:\gse2_bkte\agent\etc\.proc`）与 `validate()`（Interval 非空可 `time.ParseDuration`、QpsLimit>=0），并在 `CrontabConfig.trySetDefault()/validate()` 串接调用（与既有 `SyncCmdbGse` 一致处）。注释仅解释业务约束（全局限流粒度、缺省脚本对标 gsekit）。验证：`gofmt` + `go build ./pkg/cc/...`（如有 cc 单测则 `go test ./pkg/cc/...`）。

- [ ] T002 [FR-013] 在 `cmd/data-service/etc/data_service.yaml` 的 `crontab` 下新增 `checkProcessManaged` 样例（`enabled: false`、`interval: 20m`、`qpsLimit: 80`、`linuxProcScript`/`windowsProcScript` 缺省值），保持纯新增向后兼容。验证：`go build ./pkg/cc/...` 加载样例不报错。依赖 T001。

---

## Phase 3：用户故事 1 — 定时托管配置一致性检查与异常记录（P0）

**故事目标**：定时巡检全业务进程实例的 GSE 托管配置一致性，以 `ManagedStatus` 为应托管基准、按 `contact==GSEKIT_BIZ_{bizID}` 过滤本业务托管项，对 9 字段做子集比对识别异常类别并写入上游异常记录；单业务/单主机/单进程失败不阻断其余；仅 master 执行。
**独立测试**：以 `samples/proc-example.json` 为 `.proc` Screen 基准（含 `GSEKIT_BIZ_` 与 `nodeman` 混合项），mock GSE 返回各类 Screen / 错误码，对单业务遍历下的「写异常」路径与「单主机失败不阻断其余」独立验证；并以单包单测覆盖 `.proc` 解析（含 contact 过滤）与逐项比对判定（AC-001/AC-002/AC-004/AC-T01/AC-T02/AC-T03）。

- [ ] T003 [P] [US1] [FR-004/FR-007/AC-T02/AC-T03/SC-005/SC-006] 写失败测试 `internal/processor/processcheck/parse_test.go`（TDD 红，基准 `samples/proc-example.json`）：① 空 Screen → 返回解析失败信号（`PARSING_FAILED` 语义）；② 非 JSON 文本 → 解析失败信号；③ 正常驼峰 `{"proc":[{...},{...}]}` → `[]ActualProc`（字段映射见 data-model §2.2，含 `valuekey`/`procName`/`setupPath`/`pidPath`/`user`/5 个 `*Cmd`）；④ 按 `contact == GSEKIT_BIZ_{bizID}` 过滤本业务托管项（样例中 2 条 `nodeman` 项被剔除、3 条 `GSEKIT_BIZ_100148` 保留，SC-006）；⑤ Screen 含 "agent not available" 类信号 → 返回 agent 异常信号（`AGENT_EXCEPTION` 语义）。验证：`go test ./internal/processor/processcheck/...` 此时应失败（缺函数/类型）。

- [ ] T004 [US1] [FR-003/FR-004/FR-007] 实现 `internal/processor/processcheck/parse.go`（TDD 绿）：定义 `ActualProc` 结构（data-model §2.2 字段 + 驼峰 JSON tag）；解析函数用正则 `\{.*\}`（DOTALL）从 Screen 抽取首个 JSON 对象 → 反序列化 `{"proc":[...]}` → 按 `contact==GSEKIT_BIZ_{bizID}` 过滤本业务项；区分返回「解析失败」与「agent 异常」两类 host 级信号（对标 gsekit `_parse_ip_logs` 的 `if proc.get("contact") == self.contact`）。注释仅解释业务约束（contact 过滤、两类信号区分、剔除 versionCmd/healthCmd/GSE 内部字段不进比对）。验证：`gofmt` + `go test ./internal/processor/processcheck/...` 全绿。依赖 T003。

- [ ] T005 [US1] [FR-006/FR-007] 实现 `internal/processor/processcheck/expected.go`：定义 `ExpectedProc` 结构（data-model §2.1 字段，含 `ManagedStatus` + 9 比对字段 + 定位字段）；由 `Process`+`ProcessInstance`+`ProcessInfo`（取自 `Process.Spec.SourceData` 反序列化）调 `internal/processor/gse.BuildProcessOperate(BuildProcessOperateParams{BizID, Alias, FuncName=Process.Spec.FuncName, HostInstSeq, ...})` 渲染，映射 `Identity.ProcName(=FuncName)/SetupPath/PidPath/User` 与 `Control.{Start/Stop/Restart/Reload/Kill}Cmd`（`FaceStopCmd→KillCmd`）到 `ExpectedProc` 的 9 字段，valuekey 用 `gse.BuildNamespace(bizID)` + `BuildProcessName(alias, hostInstSeq)` 构造为 `GSEKIT_BIZ_{bizID}:{alias}_{hostInstSeq}`（**用别名 alias**）；**仅构造 9 字段**，不构造 versionCmd/healthCmd 及 GSE 内部字段；单进程渲染失败仅 `logs.Errorf`+跳过该项不阻断（对标 `buildBizOperateItems`）。验证：`gofmt` + `go build ./internal/processor/processcheck/...`。依赖 T004（同包，复用 ActualProc 不冲突）。

- [ ] T006 [P] [US1] [FR-005/FR-006/FR-007/AC-001/AC-T03/SC-001/SC-006] 写失败测试 `internal/processor/processcheck/compare_test.go`（TDD 红，基准 `samples/proc-example.json`）：构造 `ExpectedProc`/`ActualProc` 样本验证 data-model §3 判定。host 级非法项：① `actual_keys - expected_keys` 非空 → 相关实例 `ILLEGAL_VALUE_KEY`（`error_msg` 含非法 keys 集合）。按 `ManagedStatus` 分支：② `managed` + actual 缺该 valuekey → `EXPECTATION_MISMATCH`（"已托管未获取到信息"）；③ `unmanaged`/`""` + actual 存在 → `EXPECTATION_MISMATCH`（"未托管获取到信息"）；④ `unmanaged`/`""` + actual 无 → 通过不记录；⑤ `starting`/`stopping` → 跳过（skip，无写入）；⑥ `partly_managed` → 忽略。9 字段子集比对：⑦ `managed` + actual 存在且 9 字段一致 → 通过；⑧ 任一字段差异 → `EXPECTATION_MISMATCH`（`error_msg` 列出差异字段名集合，对标 gsekit `proc.items() <= actual.items()`）。验证：`go test ./internal/processor/processcheck/...` 此时应失败。依赖 T004。

- [ ] T007 [US1] [FR-005/FR-006/FR-008] 实现 `internal/processor/processcheck/compare.go`（TDD 绿）：定义 `CheckResult` 结构（data-model §2.3 字段，含 `Verdict` exception/pass/skip + `ErrorType`/`ErrorMsg`/`HandlingSuggestion`/`CheckedAt`）；按 data-model §3 规则——先 host 级 `illegal_keys` 判定（`ILLEGAL_VALUE_KEY`），再逐实例按 `ManagedStatus` 分支（managed/unmanaged|空/starting|stopping/partly_managed），managed 且 actual 存在时取 `procName/setupPath/pidPath/user/startCmd/stopCmd/restartCmd/reloadCmd/killCmd` **9 字段子集比对**，差异字段集合写入 `ErrorMsg`；异常类别归入上游 5 枚举（`PARSING_FAILED`/`AGENT_EXCEPTION`/`ILLEGAL_VALUE_KEY`/`EXPECTATION_MISMATCH`/`OTHER`，FR-008/research D5），处理建议对标 gsekit 文案。注释仅解释业务约束（子集语义、非法 valuekey host 级处理、ManagedStatus 基准）。验证：`gofmt` + `go test ./internal/processor/processcheck/...` 全绿。依赖 T006。

- [ ] T008 [P] [US1] [FR-009/FR-010/AC-001/SC-001] 写失败测试 `internal/processor/processcheck/record_test.go`（TDD 红，写入路径）：fake `dao.ProcessManagedException`，验证异常结论 → 调用 `Create`，写入字段完整（`error_type`/`error_msg`/`handling_suggestion`/`status=exception`/`checked_at` + 定位 `tenant_id`/`biz_id`/`host_id`/`process_id`/`process_instance_id`，其中 `host_id` 取自所属 `Process.Attachment.HostID`，因 `process_instances` 表不含 host_id）。验证：`go test ./internal/processor/processcheck/...` 此时应失败。依赖 T007。

- [ ] T009 [US1] [FR-009/FR-010] 实现 `internal/processor/processcheck/record.go`（TDD 绿，写入路径）：`CheckResult` → 上游 DAO 调用决策函数；`Verdict==exception` 分支调 `Create` 追加 `status=exception` 记录（字段映射见 data-model §1/§2.3）；`Verdict==pass` 分支本步留 no-op 占位（恢复闭环在 US2 T017 补齐）；`Verdict==skip` 无任何写入。host 级错误结论按实例粒度写入（扇出由 checker 提供实例集合）。验证：`gofmt` + `go test ./internal/processor/processcheck/...` 全绿。依赖 T008。

- [ ] T010 [US1] [FR-003] 实现 `internal/processor/processcheck/executor.go`：定义最小可测 seam `ScriptRunner` 接口（按 agentID + OS 下发 `cat .proc` → 返回 Screen 文本/错误，research D1/D11）；实现体内构造仅填充 `GseService/GseConf:cc.G().GSE/TaskConf:cc.G().TaskFramework` 的 `common.Executor`，复用 `WaitExecuteScriptFinish` 轮询，下发结构沿用 `config_check.go` 的 `gse.ExecuteScriptReq`（单 Agent + 单 Script + 单 AtomicTask，ScriptContent 取配置脚本、账户复用 `config.GetExecutionUser`、ScriptStoreDir 复用 `ScriptStoreDirByFileMode`、命令用 `BuildScriptCommand`）；**不**复用 istep step/callback 流水线（research D1）。验证：`gofmt` + `go build ./internal/processor/processcheck/...`。依赖 T004。

- [ ] T011 [US1] [FR-002/FR-008/FR-010/FR-012/FR-014] 实现 `internal/processor/processcheck/checker.go`：单业务编排——`Process().ListProcessesWithInstance(kit, bizID)` + `ProcessInstance().GetByProcessIDs(kit, bizID, processIDs)` 取数（对标 `SyncSingleBiz`）→ 构造期望项（T005）→ 按 agentID 分组（一个 agent 一次 `cat .proc` 拿全 host 项）→ 经 `rateLimiter.Wait(ctx)` + 信号量并发上限（对标 `sync_gse.go`）经 `ScriptRunner` 下发 → 解析（T004，含 contact 过滤）→ 比对（T007，含 host 级 illegal + ManagedStatus 分支 + 9 字段子集）→ host 级错误（解析失败/agent 异常）扇出到该 agent 下全部相关进程实例各记对应 `error_type`（FR-008/FR-010，对标 gsekit `_add_error`）→ 逐实例落库（T009）。单业务/单主机/单进程任一环节失败仅 `logs.Errorf`+`continue`，不阻断其余（FR-012）。验证：`gofmt` + `go build ./internal/processor/processcheck/...`。依赖 T005、T007、T009、T010。

- [ ] T012 [P] [US1] [AC-001/AC-002/AC-T02/AC-T03/SC-001/SC-002/SC-005/SC-006] 集成测试 `internal/processor/processcheck/checker_test.go`：fake `ScriptRunner`（返回 `samples/proc-example.json` 正常 Screen / 非 JSON / "agent not available" / 含非法 valuekey 等各类样本）+ fake `dao.ProcessManagedException`，验证：① 差异实例写一条 `exception`（AC-001/SC-001）；② 某主机解析失败 → 该主机实例记 `PARSING_FAILED`、其余主机/实例检查继续完成且正常落库（AC-002/AC-T02/SC-002/SC-005）；③ 非法 valuekey → 相关实例记 `ILLEGAL_VALUE_KEY`，`nodeman` 项不误报（AC-T03/SC-006）；④ 单进程渲染/单主机下发失败不 panic、不阻断其余（FR-012）。验证：`go test ./internal/processor/processcheck/...` 全绿。依赖 T011。

- [ ] T013 [US1] [FR-001/FR-002/FR-013/FR-014/AC-004/AC-P01/AC-T01/SC-004/SC-007] 新增定时任务入口 `cmd/data-service/service/crontab/check_managed_process.go`：`NewCheckManagedProcess(set dao.Set, sd serviced.Service, gseSvc gse.Service, cfg cc.CheckProcessManagedConfig)` + `Run()`（`time.NewTicker(interval)` + `shutdown.AddNotifier()` + `select{notifier.Signal / ticker.C}`，`ticker.C` 分支先 `if !state.IsMaster() { continue }`，对标 `sync_cmdb.go`，AC-004/AC-T01/SC-004）；持有全局 `rate.NewLimiter(rate.Limit(cfg.QpsLimit), 1)`（AC-P01/SC-007）；每轮经 `App().ListBizTenantMap(kit)` 取 `biz→tenant` 映射，逐 biz 以对应租户上下文（`kit` 克隆设 TenantID + `InternalRpcCtx()`，对标 `sync_biz_host.go`）调用 checker（T011），业务间串行；单 biz 失败 `logs.Errorf`+`continue`。验证：`gofmt` + `go build ./cmd/data-service/...`。依赖 T002、T011。

- [ ] T014 [US1] [FR-001/FR-013] 在 `cmd/data-service/app/app.go` 的 `startCronTasks()` 中按 `crontabConfig.CheckProcessManaged.Enabled` 守卫启动新巡检任务（`NewCheckManagedProcess(...).Run()`，注入 `ds.daoSet`/`ds.sd`/`ds.gseSvc`），与既有 crontab 任务接入方式一致；未开启时不影响既有行为。验证：`gofmt` + `go build ./cmd/data-service/...`。依赖 T013。

- [ ] T015 代码审查（US1）
  - 前置：调用 `superpowers:verification-before-completion` 运行 `gofmt -l pkg/cc internal/processor/processcheck cmd/data-service/service/crontab cmd/data-service/app`（输出为空）、`go test ./internal/processor/processcheck/... ./pkg/cc/...`、`go build ./...`
  - 调用 `superpowers:requesting-code-review` 审查 T001~T014 变更（配置 / 解析+contact 过滤 / 期望渲染复用 ProcName=FuncName / 9 字段子集比对 + ManagedStatus 分类 / illegal valuekey / 写异常决策 / ScriptRunner seam / 单业务编排 / 定时任务接入）
  - 重点核对：以 `ManagedStatus` 为应托管基准、9 字段裁剪（剔除 versionCmd/healthCmd 及 GSE 内部字段）、procName 来源 `Process.Spec.FuncName`、`contact==GSEKIT_BIZ_{bizID}` 过滤、valuekey 用别名 alias、host 级 illegal=actual-expected、IsMaster 守卫（slave 不下发脚本）、host 级错误扇出到全部实例、错误隔离 `continue` 不阻断、全局 rateLimiter + 并发上限、不复用 istep 流水线、复用 `BuildProcessOperate` 渲染、异常枚举映射对标 gsekit
  - Critical/Important → 停等用户指令；Minor/无问题 → 自动继续

---

## Phase 4：用户故事 2 — 异常自动恢复（闭环）（P1）

**故事目标**：已记录的异常在后续检查恢复一致时自动置为「已恢复」，无需人工逐条清理。
**独立测试**：构造「上轮 exception + 本轮一致」「上轮无记录 + 本轮一致」「上轮 recovered」样本，mock 上游 DAO 的 `IsException`/`GetLatestByProcessInstanceID`/`UpdateStatus`，独立验证恢复闭环判定与写入路径（AC-003/AC-T04）。

- [ ] T016 [P] [US2] [FR-011/FR-012/AC-T04/SC-003] 扩展失败测试 `internal/processor/processcheck/record_test.go`（TDD 红，恢复路径）：fake `dao.ProcessManagedException`，验证：① 通过（`Verdict==pass`）且 `IsException(biz, instID)`==true → 调 `GetLatestByProcessInstanceID` 取最新记录 id 后 `UpdateStatus(biz, latest.ID, recovered)`；② 通过且最新记录非 exception（无记录或已 recovered）→ 不产生 `Create`/`UpdateStatus`（no-op）；③ `UpdateStatus` 返回 error → 仅记日志不 panic、不影响其余实例（FR-012）。验证：`go test ./internal/processor/processcheck/...` 此时应失败（恢复分支尚为 no-op）。依赖 T009。

- [ ] T017 [US2] [FR-011/FR-012/AC-003/AC-T04/SC-003] 在 `internal/processor/processcheck/record.go` 补齐恢复决策（TDD 绿）：`Verdict==pass`（通过）分支调 `IsException(biz, instID)`，为 true 时 `GetLatestByProcessInstanceID` + `UpdateStatus(recovered)` 完成闭环，否则不动作（以"最近一次检查结论"为准）；`UpdateStatus` 失败仅 `logs.Errorf` 并在下一轮重试（不阻断其余实例）。验证：`gofmt` + `go test ./internal/processor/processcheck/...` 全绿。依赖 T016。

- [ ] T018 [P] [US2] [AC-003/SC-003] 扩展集成测试 `internal/processor/processcheck/checker_test.go`：fake `ScriptRunner`（返回与期望一致的 `.proc` Screen）+ fake DAO（`IsException` 返回 true），验证「上轮 exception + 本轮一致」经单业务编排后该实例最新记录 `UpdateStatus(recovered)`；并验证「上轮无记录/已 recovered + 本轮一致」无多余写入。验证：`go test ./internal/processor/processcheck/...` 全绿。依赖 T017、T012。

- [ ] T019 代码审查（US2）
  - 前置：调用 `superpowers:verification-before-completion` 运行 `gofmt` + `go test ./internal/processor/processcheck/...` + `go build ./...`
  - 调用 `superpowers:requesting-code-review` 审查 T016~T018 变更（恢复决策 + 恢复闭环集成）
  - 重点核对：恢复以"最新记录为准"（`IsException`→`GetLatestByProcessInstanceID`→`UpdateStatus`）、非 exception 不动作、`UpdateStatus` 失败仅记日志下一轮重试、不阻断其余实例
  - Critical/Important → 停等用户指令；Minor/无问题 → 自动继续

---

## Phase 5：收尾与跨切面（Polish）

- [ ] T020 [P] [FR-015] 评审核对：写入异常记录的字段（`error_type`/`error_msg`/`handling_suggestion` + 定位字段）仅含运维类信息（路径/命令/账户名/差异字段名），不落敏感个人信息；确认日志输出同样不含敏感信息（对标 `.cursor/skills/bk-security-redlines`）。
- [ ] T021 全量验证：`gofmt -l pkg/cc internal/processor/processcheck cmd/data-service/service/crontab cmd/data-service/app`（输出为空）、`go test ./internal/processor/processcheck/... ./pkg/cc/...`（全绿）、`go build ./...`（通过）。

---

## 依赖与并行

- **完成顺序**：Phase 2 → Phase 3（US1，P0，构成最小可用切片）→ Phase 4（US2，P1，补齐恢复闭环）→ Phase 5。
- **Phase 2 内**：T001 → T002（yaml 样例依赖配置结构定稿）。
- **Phase 3 内**：T003 [P]（解析测试）先于 T004；T004 后 T005 / T006 [P] / T010 可并行展开（均依赖 ActualProc 但互不冲突，注意同包顺序提交）；T006 [P] 先于 T007；T008 [P] 先于 T009；T011 依赖 T005+T007+T009+T010；T012 [P] 依赖 T011；T013 依赖 T002+T011；T014 依赖 T013。
- **Phase 4 内**：T016 [P] 先于 T017；T018 [P] 依赖 T017+T012。
- **并行示例**：T003 / T006 / T008（三个测试文件独立）可在各自前置满足后并行编写；T020 [P] 评审项可与 T021 前并行准备。
- **MVP 范围**：Phase 2 + Phase 3（US1）即构成「检查—记录」最小可用切片；Phase 4（US2）补齐「恢复」闭环。

## FR / SC / AC 覆盖核对

| 编号 | 含义 | 落点任务 |
|------|------|---------|
| FR-001 | 定时仅 master 执行 | T013、T014 |
| FR-002 | 跨租户按业务遍历 | T011（单业务取数）、T013（ListBizTenantMap） |
| FR-003 | 复用脚本执行构建块（cat .proc）| T010、T004（Screen 解析） |
| FR-004 | `.proc` 脚本可配 + 解析失败 PARSING_FAILED | T001（配置）、T003、T004 |
| FR-005 | ManagedStatus 分支判定 | T006、T007 |
| FR-006 | 9 字段子集比对（procName=FuncName）| T005、T006、T007 |
| FR-007 | contact 过滤 + valuekey + 非法项 | T003、T004、T005、T006、T007 |
| FR-008 | 异常类别→上游 5 枚举映射 | T007、T011 |
| FR-009 | 写 exception 记录 | T008、T009 |
| FR-010 | 实例粒度 + host 扇出 | T009、T011 |
| FR-011 | 恢复闭环 | T016、T017 |
| FR-012 | 单点失败不阻断 | T011、T012、T016、T017 |
| FR-013 | 巡检子配置 | T001、T002、T013、T014 |
| FR-014 | 限流 / 并发上限 | T011、T013 |
| FR-015 | 不落敏感信息 | T020、T015（评审） |
| SC-001 | 100% 产 EXPECTATION_MISMATCH 记录（字段完整）| T006、T007、T009、T012 |
| SC-002 | 失败范围准确记录、其余继续 | T011、T012 |
| SC-003 | 下一轮翻转 recovered | T017、T018 |
| SC-004 | slave 不下发脚本 | T013 |
| SC-005 | 解析失败记 PARSING_FAILED 不阻断 | T003、T004、T011、T012 |
| SC-006 | nodeman 不误报 + 非法项必记 | T003、T004、T006、T007、T012 |
| SC-007 | 单轮巡检 GSE 调用受限流 | T011、T013 |
| AC-001 | 写异常字段完整 | T006、T007、T009、T012 |
| AC-002 | 单主机失败不阻断 | T011、T012 |
| AC-003 | 恢复闭环 | T017、T018 |
| AC-004 / AC-T01 | slave 跳过 | T013 |
| AC-P01 | 限流 | T011、T013 |
| AC-T02 | 解析失败 | T003、T004、T011、T012 |
| AC-T03 | 非法 valuekey + contact 过滤 | T003、T006、T007、T012 |
| AC-T04 | 恢复 recovered | T016、T017、T018 |
