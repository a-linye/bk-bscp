# 任务清单：进程托管异常记录数据存储

**需求 ID**：短 ID 135663687 / 长 ID 1020451610135663687
**输入**：`plan.md`、`research.md`、`data-model.md`、`spec.md`（FR-001~FR-012、AC-001~004、AC-T01/T02）
**模式**：测试驱动开发（TDD）——可单包验证的部分先写测试再实现
**上下文白名单**：见 `context.md`（仅在 Code scope 内改动）

## 说明

- 本子需求为纯数据层（新增表 + DAO），无项目初始化任务，故无独立 Setup 阶段。
- 按仓库标准数据层依赖顺序排列：migration → table 模型/枚举 → gen 生成 → DAO 实现 → Set 挂载。Phase 2（基础链路）为两条用户故事的共同前置；Phase 3（US1 写入/历史）与 Phase 4（US2 判定/恢复）分别落地各自 DAO 方法。
- **测试取舍（plan-report.md A1 / research.md D9）**：`pkg/dal/table` 枚举/结构以单包单测强约束（TDD 必做）；`internal/dal/dao` 包无 sqlmock/sqlite 基建且 `set_tenant_id` 回调依赖 `cc.G()` 全局配置，DAO 行为类验收（AC-001~004、AC-T01/T02）标注为**集成环境 + 代码评审**验证，并以 `var _ ProcessManagedException = new(processManagedExceptionDao)` 编译期接口断言兜底。
- migration 版本号需大于现存最新（`20260528170000`）。下文以 `20260630173000` 占位，实现时取生成时刻递增时间戳。
- `[P]` 标记表示可与同 Phase 内其他 `[P]` 任务并行（不同文件、无未完成依赖）。

---

## Phase 2：基础链路（Foundational — 阻塞所有用户故事，必须先完成）

- [ ] T001 [FR-001/FR-010] 新增建表 migration `cmd/data-service/db-migration/migrations/20260630173000_add_process_managed_exception.go`：`init()` 注册 `GormMode` migration；Up 内联定义 `ProcessManagedException` 结构（5 业务字段 + 5 定位字段 + 4 Revision 字段，gorm tag 含列名/类型/联合索引 `idx_bizID_processInstanceID(biz_id priority:1, process_instance_id priority:2)`），`AutoMigrate` 建表 + `tx.Create([]IDGenerators{{Resource:"process_managed_exceptions", MaxID:0, UpdatedAt: now}})`；Down 删除 `id_generators` 中 `process_managed_exceptions` 资源 + `DropTable("process_managed_exceptions")`。对标 `20250923114014_add_process.go`。验证：`go build ./cmd/data-service/...`。

- [ ] T002 [FR-001] 在 `pkg/dal/table/table.go` 新增表名常量 `ProcessManagedExceptionsTable Name = "process_managed_exceptions"`。验证：`go build ./pkg/dal/table/...`。

- [ ] T003 [P] [FR-004/FR-005] 写失败测试 `pkg/dal/table/process_managed_exception_test.go`（TDD 红）：① `ProcessExceptionErrorType.Validate()` 五值（`PARSING_FAILED`/`AGENT_EXCEPTION`/`ILLEGAL_VALUE_KEY`/`EXPECTATION_MISMATCH`/`OTHER`）各通过、非法值返回 error；② `ProcessExceptionStatus.Validate()` `exception`/`recovered` 通过、非法值返回 error；③ `(*ProcessManagedException).TableName()` == `table.ProcessManagedExceptionsTable`；④ 结构断言 `ProcessManagedExceptionAttachment` 含 `TenantID` 字段（AC-T01 复用回调前提）。验证：`go test ./pkg/dal/table/...` 此时应失败（缺类型/方法）。依赖 T002。

- [ ] T004 [FR-001/FR-002/FR-003/FR-004/FR-005] 实现 `pkg/dal/table/process_managed_exception.go`（TDD 绿）：三段式模型 `ProcessManagedException{ID + Attachment(embedded) + Spec(embedded) + Revision(embedded)}`；`ProcessManagedExceptionSpec`（ErrorType/ErrorMsg/HandlingSuggestion/Status/CheckedAt，列名与类型见 data-model §2.1）；`ProcessManagedExceptionAttachment`（TenantID/BizID/HostID/ProcessID/ProcessInstanceID，见 §2.2）；两个 string 枚举 `ProcessExceptionErrorType`/`ProcessExceptionStatus` + 各自常量 + `Validate()`（风格对齐 `process_instance.go` 的 `ProcessStatus.Validate()`）；`TableName()` 返回 `ProcessManagedExceptionsTable`。注释仅解释业务约束（host_id 取自 ProcessAttachment、命名前缀防冲突）。验证：`gofmt` + `go test ./pkg/dal/table/...` 全绿。依赖 T003。

- [ ] T005 [FR-011] 在 `scripts/gen/main.go` 的 `g.ApplyBasic(...)` 末尾追加 `table.ProcessManagedException{}`，执行 `make gen` 生成 `internal/dal/gen/` 产物（`process_managed_exception.gen.go` 及 query 注册），**不手改生成物**。验证：`git diff internal/dal/gen` 仅含新增；`go build ./internal/dal/gen/...`。依赖 T004。

- [ ] T006 [FR-006~FR-009/FR-011] 新增 DAO 接口骨架 `internal/dal/dao/process_managed_exception.go`：定义接口 `ProcessManagedException`（5 方法签名见 data-model §5）与实现体 `processManagedExceptionDao{ genQ, idGen, auditDao }`（对标 `config_template.go`/`process.go`），加入编译期断言 `var _ ProcessManagedException = new(processManagedExceptionDao)`（方法体可先返回零值/未实现，留待 T008/T010 填充）。验证：`go build ./internal/dal/dao/...`。依赖 T005。

- [ ] T007 [FR-011] 在 `internal/dal/dao/dao.go` 的 `Set` 接口新增 `ProcessManagedException() ProcessManagedException`，并在 `set` 结构体新增工厂方法返回 `&processManagedExceptionDao{idGen: s.idGen, auditDao: s.auditDao, genQ: s.genQ}`（对标既有 `Process()`/`ConfigTemplate()` 工厂）。验证：`go build ./internal/dal/...`（编译期校验接口实现）。依赖 T006。

- [ ] T008 代码审查（Phase 2）
  - 前置：调用 `superpowers:verification-before-completion` 运行 `gofmt -l pkg/dal/table internal/dal/dao scripts cmd/data-service/db-migration`、`go test ./pkg/dal/table/...`、`go build ./...`
  - 调用 `superpowers:requesting-code-review` 审查 T001~T007 变更（migration / table 模型 / gen 注册 diff / DAO 骨架 / Set 挂载）
  - 重点核对：联合索引列序、枚举取值与 gsekit 对标、TenantID 字段命名（回调依赖）、生成物未手改、FR-012 仅运维字段
  - Critical/Important → 停等用户指令；Minor/无问题 → 自动继续

---

## Phase 3：用户故事 1 — 异常记录写入与历史保留（P1）

**故事目标**：检查侧可追加写入异常记录、按进程实例查询全部历史明细（非覆盖）。
**独立测试**：在带 DB 集成环境写入一条/多条记录并查询，验证字段完整（AC-001）与历史全返回（AC-002）。

- [ ] T009 [US1] [FR-006/AC-001] 在 `internal/dal/dao/process_managed_exception.go` 实现 `Create(kit *kit.Kit, m *table.ProcessManagedException) (uint32, error)`：`idGen.One(kit, table.ProcessManagedExceptionsTable)` 分配 ID → `genQ.ProcessManagedException.WithContext(kit.Ctx).Create(m)`；不挂审计（D7）；写库失败直接返回 error（不重试/吞错，边界场景"写库失败"）。验证：`go build ./internal/dal/dao/...`；AC-001 行为在集成环境 + 评审验证。依赖 T007。

- [ ] T010 [US1] [FR-007/AC-002] 实现 `ListByProcessInstanceID(kit, bizID, processInstanceID uint32) ([]*table.ProcessManagedException, error)`：`Where(biz_id=? AND process_instance_id=?).Order(id desc).Find()`；租户隔离由 `set_tenant_id` 回调自动追加 `tenant_id=?`。验证：`gofmt` + `go build ./internal/dal/dao/...`；AC-002 历史全返回在集成环境 + 评审验证。依赖 T009（同文件，顺序实现）。

- [ ] T011 代码审查（US1）
  - 前置：调用 `superpowers:verification-before-completion` 运行 `gofmt` + `go build ./internal/dal/...`
  - 调用 `superpowers:requesting-code-review` 审查 T009~T010 变更
  - 重点核对：Create 走 idGen.One、非覆盖追加语义、List 走联合索引（Order id desc）、租户隔离不被绕过
  - Critical/Important → 停等用户指令；Minor/无问题 → 自动继续

---

## Phase 4：用户故事 2 — 异常态判定与恢复（P1）

**故事目标**：以"最新一条记录状态"判定进程实例当前是否异常，并支持将目标记录置为已恢复。
**独立测试**：构造"有异常""恢复后""无记录""最新为已恢复"样本，验证判定与状态更新（AC-003/AC-004/AC-T02）。

- [ ] T012 [US2] [FR-008/AC-T02] 在 `internal/dal/dao/process_managed_exception.go` 实现 `GetLatestByProcessInstanceID(kit, bizID, processInstanceID uint32) (*table.ProcessManagedException, error)`：`Where(biz_id=? AND process_instance_id=?).Order(id desc).Take()`，无记录返回 `gorm.ErrRecordNotFound`。验证：`go build ./internal/dal/dao/...`；以最新记录为准的行为在集成环境 + 评审验证。依赖 T007。

- [ ] T013 [US2] [FR-008/AC-004/AC-T02] 实现 `IsException(kit, bizID, processInstanceID uint32) (bool, error)`：调用 `GetLatestByProcessInstanceID`；`errors.Is(err, gorm.ErrRecordNotFound)` → 返回 `false, nil`；其他 err 透传；否则返回 `latest.Spec.Status == table.ProcessExceptionStatusException, nil`。验证：`go build ./internal/dal/dao/...`；可对"无记录→false"分支做轻量断言兜底（A1）。依赖 T012。

- [ ] T014 [US2] [FR-009/AC-003] 实现 `UpdateStatus(kit, bizID, id uint32, status table.ProcessExceptionStatus) error`：`Where(biz_id=? AND id=?).Updates({status, reviser=kit.User, updated_at=time.Now()})`（仅恢复语义，刷新 reviser/updated_at，历史明细保留）。验证：`gofmt` + `go build ./internal/dal/dao/...`；AC-003 恢复后判定翻转在集成环境 + 评审验证。依赖 T013（同文件，顺序实现）。

- [ ] T015 代码审查（US2）
  - 前置：调用 `superpowers:verification-before-completion` 运行 `gofmt` + `go build ./internal/dal/...`
  - 调用 `superpowers:requesting-code-review` 审查 T012~T014 变更
  - 重点核对：GetLatest 走 `Order(id desc).Take()` 命中索引、IsException 的 ErrRecordNotFound→false 分支、UpdateStatus 仅改 status/reviser/updated_at 且不覆盖历史
  - Critical/Important → 停等用户指令；Minor/无问题 → 自动继续

---

## Phase 5：收尾与跨切面（Polish）

- [ ] T016 [P] [FR-012] 评审核对：异常记录列（error_type/error_msg/handling_suggestion 等）仅含运维类信息，不落敏感个人信息；确认 migration 列定义与 data-model §2 一致。
- [ ] T017 全量验证：`gofmt -l pkg/dal/table internal/dal/dao scripts cmd/data-service/db-migration`（输出为空）、`go test ./pkg/dal/table/...`（全绿）、`make gen` 后 `git diff internal/dal/gen`（仅本期新增）、`go build ./...`（通过）。

---

## 依赖与并行

- **完成顺序**：Phase 2 → (Phase 3 ∥ Phase 4，二者均仅依赖 Phase 2；但同文件 `process_managed_exception.go` 的方法实现需顺序写入避免冲突) → Phase 5。
- **Phase 2 内**：T001、T002 可并行（不同文件）；T003 依赖 T002；T004 依赖 T003；T005 依赖 T004；T006 依赖 T005；T007 依赖 T006。
- **并行示例**：T001 [P] 与 T002 可同时进行；T003 [P]（测试文件独立）；T016 [P] 评审项可与 T017 前并行准备。
- **MVP 范围**：Phase 2 + Phase 3（US1，写入与历史）即构成最小可用数据写入/查询切片；Phase 4（US2，判定与恢复）补齐异常闭环。

## FR / AC 覆盖核对

| 编号 | 落点任务 |
|------|---------|
| FR-001 新表三段式 | T001、T002、T004 |
| FR-002 业务字段 | T004 |
| FR-003 定位字段 + tenant 回调 | T004（TenantID 字段）、T007（复用回调） |
| FR-004 error_type 枚举 + Validate | T003、T004 |
| FR-005 status 枚举 + Validate | T003、T004 |
| FR-006 写入追加 + idGen | T009 |
| FR-007 按实例/业务查询 | T010 |
| FR-008 当前是否异常判定 | T012、T013 |
| FR-009 状态更新恢复 | T014 |
| FR-010 联合索引 | T001 |
| FR-011 gen 注册 + Set 挂载 | T005、T006、T007 |
| FR-012 不落敏感信息 | T004、T016 |
| AC-001 写入字段完整 | T001、T004、T009 |
| AC-002 历史非覆盖 | T010 |
| AC-003 恢复后判定翻转 | T013、T014 |
| AC-004 无记录/最新 recovered → 否 | T013 |
| AC-T01 tenant 自动填充 + 隔离 | T003（字段断言）、T007（复用回调） |
| AC-T02 最新记录为准 | T012、T013 |
