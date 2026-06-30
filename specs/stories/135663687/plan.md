# 实现计划：进程托管异常记录数据存储

**需求 ID**：短 ID 135663687 / 长 ID 1020451610135663687
**分支**：不新建分支（在当前工作分支实现）
**输入**：`spec.md`（FR-001~FR-012、AC-001~004、AC-T01/T02）、`research.md`、`data-model.md`
**模式**：测试驱动开发（TDD）—— 可独立单包验证的部分先写测试再实现

## 摘要

为父需求"GSE 托管信息检查"提供数据层：新增独立 MySQL 表 `process_managed_exceptions` 与对应 DAO（写入追加 / 按进程实例·业务查历史 / 取最新判定"当前是否异常" / 状态更新为 recovered）。复用仓库标准数据层链路（migration → table 模型 → gen 生成 → DAO → Set 挂载），无新引入框架，纯新增向后兼容。

## 技术上下文（Technical Context）

| 项 | 取值 |
|----|------|
| 语言 / 运行时 | Go（仓库现有版本，见 `go.mod`） |
| 存储 | MySQL（bscp data-service DB），GORM + gorm/gen |
| 主要依赖 | `gorm.io/gorm`、`gorm.io/gen`、仓库 `internal/dal/{gen,dao,sharding}`、`pkg/dal/table`、`pkg/kit` |
| 迁移框架 | `cmd/data-service/db-migration/migrator`（GormMode） |
| ID 分配 | `internal/dal/dao/id.go` `IDGenInterface.One/Batch` + `id_generators` 表 |
| 租户隔离 | `internal/dal/dao/set_tenant_id.go` 既有回调（复用，不改） |
| 测试 | 表层纯单测（`go test ./pkg/dal/table/...`）；DAO 行为依赖带 DB 环境（见 research.md D9） |
| 新增外部依赖 | 无 |
| 待澄清项 | 无（Q1–Q6 均 resolved_by_doc） |

## 约束 / 合规自检基线（替代 Constitution Check）

> 本仓库无 `.specify/memory/constitution.md`，约束以 `AGENTS.md` 为准。

- [x] **语言**：代码标识符/协议字段保持英文（表名/列名/枚举沿用 gsekit + 仓库风格）；注释中文，仅解释业务约束（host_id 来源、最新记录判定、不接审计原因）。
- [x] **Go 规范**：符合 `.golangci.yml`；改动后 `gofmt`；枚举 `Validate()` 风格对齐既有；命名前缀防冲突。
- [x] **不引入不必要抽象**：不实现 AuditRes（Q4）；不建冗余 biz_id 独立索引（D3）；不新增 sqlite/sqlmock 依赖（D9）。
- [x] **生成文件**：`internal/dal/gen/` 由 `make gen` 重新生成，不手改；提交前检查 diff。
- [x] **纯新增向后兼容**：不改既有表结构与既有 DAO 行为。
- [x] **数据保护**：异常记录仅存运维类信息，不落敏感个人信息（FR-012）。

## 项目结构 / 文件级改动（按依赖顺序）

```
cmd/data-service/db-migration/migrations/
  └─ <版本号>_add_process_managed_exception.go   [新增] 建表 + 插入 id_generators 资源；Down 删表 + 删资源
pkg/dal/table/
  ├─ process_managed_exception.go                [新增] 模型 + Spec/Attachment + 两个枚举(+Validate) + TableName
  ├─ process_managed_exception_test.go           [新增] 枚举 Validate / TableName / 字段结构 单测（TDD 先行）
  └─ table.go                                     [改动] 新增 ProcessManagedExceptionsTable 常量
scripts/gen/main.go                               [改动] ApplyBasic 注册 table.ProcessManagedException{}
internal/dal/gen/                                 [生成] make gen 产物（process_managed_exception.gen.go 等，不手改）
internal/dal/dao/
  ├─ process_managed_exception.go                [新增] DAO 接口 + 实现（Create/List/GetLatest/IsException/UpdateStatus）
  └─ dao.go                                       [改动] Set 接口 + set 工厂方法新增 ProcessManagedException()
```

## Phase 0 — 调研（见 research.md）

已完成，无 NEEDS CLARIFICATION 未决项。关键决策：分层与定位字段(D1)、枚举风格(D2)、索引(D3)、ID 分配(D4)、租户回调复用(D5)、读写恢复语义(D6)、不接审计(D7)、列类型(D8)、测试基建取舍(D9)、migration 命名(D10)。

## Phase 1 — 设计（见 data-model.md）

表 `process_managed_exceptions`：5 业务字段 + 5 定位字段 + 4 Revision 字段；联合索引 `(biz_id, process_instance_id)`；两枚举 `ProcessExceptionErrorType`(五值) / `ProcessExceptionStatus`(两值)；DAO 五方法契约。

本需求为内部数据层，不对外暴露新接口/契约（无 contracts/、无 quickstart）。

## Phase 2 — TDD 实现顺序

> 每步对应 tasks 阶段一条/多条任务。可单包验证的步骤遵循"先写测试→红→实现→绿"。

1. **migration（建表，FR-001/FR-010/FR-006-ID 前提）**
   - 通过 `migrate create -n add_process_managed_exception` 生成骨架，或对标 `20250923114014_add_process.go` 手写。
   - Up：`AutoMigrate(&ProcessManagedException{})`（内嵌 gorm tag 定义列/类型/联合索引 `idx_bizID_processInstanceID`）+ `tx.Create([]IDGenerators{{Resource:"process_managed_exceptions", MaxID:0, UpdatedAt: now}})`。
   - Down：删 `id_generators` 中 `process_managed_exceptions` 资源 + `DropTable("process_managed_exceptions")`。
   - 验证：`go build ./cmd/data-service/...`（migration 文件随服务编译）。

2. **table 模型 + 枚举（FR-001~FR-005）— TDD**
   - 先写 `process_managed_exception_test.go`：
     - `ProcessExceptionErrorType.Validate()`：五值各通过；非法值返回 error（AC-T 枚举前提，FR-004）。
     - `ProcessExceptionStatus.Validate()`：`exception`/`recovered` 通过；非法值返回 error（FR-005）。
     - `(*ProcessManagedException).TableName()` == `ProcessManagedExceptionsTable`。
     - 结构断言：Attachment 含 `TenantID` 字段（AC-T01 复用回调前提，D5）。
   - 再实现 `process_managed_exception.go` 模型 + 两枚举 + `Validate()` + `TableName()`，使测试转绿。
   - 在 `table.go` 增加 `ProcessManagedExceptionsTable Name = "process_managed_exceptions"`。
   - 验证：`gofmt` + `go test ./pkg/dal/table/...`。

3. **注册 gen 模型并生成（FR-011）**
   - `scripts/gen/main.go` 的 `ApplyBasic(...)` 末尾追加 `table.ProcessManagedException{}`。
   - 执行 `make gen`，检查 `internal/dal/gen/` diff（新增 `process_managed_exception.gen.go` 与 query 注册），不手改生成物。
   - 验证：`go build ./internal/dal/gen/...`。

4. **DAO 接口 + 实现（FR-006~FR-009）**
   - 新增 `internal/dal/dao/process_managed_exception.go`：
     - 接口 `ProcessManagedException` + 实现体 `processManagedExceptionDao{ genQ, idGen, auditDao }`（对标 `config_template.go` / `process.go`）。
     - `Create`：`idGen.One` 分配 ID → `genQ.ProcessManagedException.WithContext(...).Create(m)`；不挂审计（D7）；写库失败返回 error（不吞错）。
     - `ListByProcessInstanceID`：`Where(biz_id, process_instance_id).Order(id desc).Find()`。
     - `GetLatestByProcessInstanceID`：`Where(biz_id, process_instance_id).Order(id desc).Take()`。
     - `IsException`：调用 GetLatest；`errors.Is(err, ErrRecordNotFound)` → `false,nil`；否则 `latest.Spec.Status == ProcessExceptionStatusException`（AC-004/AC-T02）。
     - `UpdateStatus`：`Where(biz_id, id).Updates({status, reviser, updated_at})`（仅恢复语义，FR-009）。
   - 验证：`gofmt` + `go build ./internal/dal/dao/...`。

5. **挂载 Set（FR-011）**
   - `dao.go`：`Set` 接口新增 `ProcessManagedException() ProcessManagedException`；`set` 新增工厂方法返回 `&processManagedExceptionDao{idGen, auditDao, genQ}`。
   - 验证：`go build ./internal/dal/...`（`var _ ProcessManagedException = new(processManagedExceptionDao)` 编译期校验接口实现）。

6. **测试收口**
   - 表层单测全绿（步骤 2）。
   - DAO 行为（AC-001~004、AC-T01/T02）：在带 DB 的集成环境验证；本期以接口实现编译期断言 + 代码评审保障（research.md D9）。

## 验收映射（计划覆盖核对）

| AC | 覆盖步骤 | FR |
|----|---------|-----|
| AC-001 写入字段完整 | 1(建表)+2(模型)+4(Create) | FR-001~FR-006 |
| AC-002 历史非覆盖 | 4(ListByProcessInstanceID, 追加写) | FR-007 |
| AC-003 恢复后判定翻转 | 4(UpdateStatus + IsException) | FR-008/FR-009 |
| AC-004 无记录/最新 recovered → 否 | 4(IsException 分支) | FR-008 |
| AC-T01 tenant 自动填充+隔离 | 2(TenantID 字段)+5(复用回调) | FR-003 |
| AC-T02 最新记录为准 | 4(GetLatest Order id desc) | FR-008 |

| FR | 落点 |
|----|------|
| FR-001 新表三段式 | 步骤 1+2 |
| FR-002 业务字段 | 步骤 2（Spec） |
| FR-003 定位字段+tenant 回调 | 步骤 2（Attachment）+5 |
| FR-004 error_type 枚举+Validate | 步骤 2 |
| FR-005 status 枚举+Validate | 步骤 2 |
| FR-006 写入追加+idGen | 步骤 4 Create |
| FR-007 按实例/业务查询 | 步骤 4 List |
| FR-008 当前是否异常判定 | 步骤 4 GetLatest/IsException |
| FR-009 状态更新恢复 | 步骤 4 UpdateStatus |
| FR-010 索引 | 步骤 1 |
| FR-011 gen 注册 + Set 挂载 | 步骤 3+5 |
| FR-012 不落敏感信息 | 步骤 2（仅运维字段）+ 评审核对 |

## 复杂度 / 风险跟踪

| 项 | 说明 | 处置 |
|----|------|------|
| DAO 单测缺基建 | dao 包无 sqlmock/sqlite，回调依赖 cc 全局配置 | 不引入新依赖；表层单测 + 集成验证 + 评审（D9），在 plan-report testability 记录 |
| 大表累积 | 历史明细线性增长（TR-001） | 联合索引保证查询；归档策略本期范围外 |
| 命名/枚举一致性 | 与后续检查侧子需求对齐（TR-002） | 已对标 gsekit + 父需求，plan 阶段定名（data-model §1/§4） |

## 验证命令汇总

```bash
gofmt -l pkg/dal/table internal/dal/dao scripts cmd/data-service/db-migration
go test ./pkg/dal/table/...
make gen   # 生成后检查 git diff internal/dal/gen
go build ./...
```
