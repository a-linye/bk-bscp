# 技术调研：进程托管异常记录数据存储

**需求 ID**：短 ID 135663687 / 长 ID 1020451610135663687
**范围**：纯数据层（新增独立 MySQL 表 + DAO），无跨系统调用。所有结论均依据 `context.md` 白名单源码 + `req.md` / `spec.md` / `questions.md`。

## D1 — 表模型分层与定位字段（FR-001、FR-003 / Q1）

- **决策**：沿用仓库三段式 `ID + Attachment(embedded) + Spec(embedded) + Revision(embedded)`；定位字段冗余存储 `tenant_id/biz_id/host_id/process_id/process_instance_id`。
- **理由**：`pkg/dal/table/process_instance.go` 的 `ProcessInstanceAttachment` 仅含 `tenant_id/biz_id/process_id/cc_process_id`，**不含 host_id**；`host_id` 位于 `pkg/dal/table/process.go` 的 `ProcessAttachment`。检查侧/操作侧需免 join 快速定位，故冗余存储。
- **备选**：仅存 `process_instance_id`、其余 join 查询 → 否决（查询需多表 join，违背性能需求）。

## D2 — 枚举风格（FR-004、FR-005 / Q2、Q3）

- **决策**：`error_type` / `status` 定义为 string 枚举类型 + `Validate()` 方法，取值与 gsekit 对标。
- **理由**：与 `process.go` 的 `AgentStatus`、`process_instance.go` 的 `ProcessStatus`/`ProcessManagedStatus` 风格完全一致（`type X string` + `const` + `Validate() error`）。`error_type` 对标 `bk-process-config-manager/apps/gsekit/process/handlers/check_process.py` 的 `ErrorType` 五值。
- **命名**：统一前缀 `ProcessException*`，避免与既有 `ProcessStatus`/`ProcessManagedStatus` 命名冲突。

## D3 — 索引设计（FR-010 / Q5）

- **决策**：建立联合索引 `idx_bizID_processInstanceID (biz_id, process_instance_id)`；不建独立 `biz_id` 索引；`tenant_id` 不单独建索引。
- **理由**：
  - 主查询为"按进程实例查历史/取最新"：`WHERE biz_id=? AND process_instance_id=? ORDER BY id DESC`，联合索引完全命中（SC-005）。
  - FR-010"保留 biz_id 维度查询能力"：联合索引最左前缀 `biz_id` 即可服务 biz_id-only 查询，无需冗余独立索引（MySQL 最左前缀原则；符合 AGENTS.md"不引入不必要"）。
  - `tenant_id` 由 `set_tenant_id` 回调追加为 WHERE 附加条件；数据先按 biz_id 收敛，无需为其单独建索引。
- **备选**：(biz_id, process_instance_id) + 独立 biz_id 索引 → 否决（冗余）。

## D4 — ID 分配（FR-006）

- **决策**：写入用 `idGen.One(kit, table.ProcessManagedExceptionsTable)`；migration 向 `id_generators` 插入资源记录 `process_managed_exceptions`。
- **理由**：`internal/dal/dao/id.go` 的 `IDGenInterface.One/Batch` 依赖 `id_generators` 表存在对应 `resource` 行；`config_template.go` 的 `CreateWithTx` 即用 `idGen.One`。migration 样板 `20250923114014_add_process.go` 在建表后 `tx.Create([]IDGenerators{{Resource:"processes"}})`，Down 删除该资源。

## D5 — 租户隔离（FR-003、AC-T01 / Q1）

- **决策**：复用既有 `internal/dal/dao/set_tenant_id.go` 回调，模型 Attachment 提供名为 `TenantID`（列 `tenant_id`）的字段即可；本需求**不重写、不重测**该通用回调。
- **理由**：回调通过 `db.Statement.Schema.LookUpField("TenantID")` 自动：写入注入 tenant_id、查询/更新/删除追加 `tenant_id=?` 过滤。该机制为全表共享的既有已验证基建（`excludedTables` 仅排除 `configs`/`id_generators`，本表不在其中）。AC-T01 通过"字段命名正确 + 复用既有回调"满足，单测仅做字段/列名结构断言，不重测框架行为（符合 AGENTS.md 不引入不必要抽象/重复）。

## D6 — 写入/查询/恢复语义（FR-006~FR-009 / Q6）

- **决策**：写入追加新行（非覆盖）；恢复 = 对目标异常记录 `UPDATE status=recovered` 并刷新 reviser/updated_at；"当前是否异常"= 取最新一条记录判定。
- **理由**：满足 AC-002（历史非覆盖）、AC-003（恢复后判定翻转）、AC-004/AC-T02（无记录或最新 recovered → 非异常）。取最新用 `Order(id desc).Take()`，自增/分配 ID 单调递增可代表时间序（与 `created_at desc` 等价，id 已建索引更优）。

## D7 — 审计（Q4 / FR 范围外）

- **决策**：不实现 `AuditRes` 接口（`ResID/ResType/AppID`），DAO 不挂 `auditDao.Decorator`。
- **理由**：异常记录由后台巡检自动写入/恢复，非用户对资源的操作变更；父需求与本需求均无审计诉求。依据 AGENTS.md"不引入不必要的抽象"。`auditDao` 字段仍按样板保留在 dao 结构体（与 `set` 工厂统一），但写入路径不调用审计。

## D8 — 列类型选择（FR-002、FR-012）

- **决策**：`error_msg` 用 `text`（差异信息长度不定）；`handling_suggestion` 用 `varchar(1024)`；`checked_at` 用 `datetime not null`；`status` 用 `varchar(32) not null default 'exception'`；`error_type` 用 `varchar(64) not null`。
- **理由**：对标 `process.go` migration 中 `source_data`(json)/`alias`(varchar) 的类型粒度习惯；异常描述不落敏感个人信息（FR-012），仅存类型/差异/建议等运维数据。

## D9 — 单元测试基建现状（测试策略 / AGENTS.md 测试优先）

- **现状**：`internal/dal/dao` 包内无 sqlmock / sqlite 测试基建（`go.mod` 无 sqlmock/sqlite 驱动；现存 `audit_builder_test.go` 为空壳）。仓库 DAO 多依赖真实 DB 集成验证。
- **决策**：
  - **表层（`pkg/dal/table`）**：新增纯单元测试，覆盖 `ProcessExceptionErrorType.Validate()`（五值通过 + 非法值报错）、`ProcessExceptionStatus.Validate()`（两值通过 + 非法值报错）、`TableName()` 返回常量、Attachment 含 `TenantID` 字段。**无需 DB**，可单包 `go test ./pkg/dal/table/...` 验证（覆盖 FR-004/FR-005，AC 的枚举与结构前提）。
  - **DAO 层（`internal/dal/dao`）**：DAO 方法逻辑（Create/List/GetLatest/IsException/UpdateStatus）的行为正确性（AC-001~004、AC-T02）依赖真实 DB 的 GORM/gen 执行。**不新增 sqlite/sqlmock 依赖**（AGENTS.md 不引入不必要依赖；且 set_tenant_id 回调依赖 `cc.G()` 全局配置，难以在纯单测内构造）。这些 AC 在带 DB 的集成环境验证；本期单测聚焦表层可独立验证部分 + `IsException` 的"无记录→false"分支可通过对 `ErrRecordNotFound` 的判定逻辑做轻量断言。
- **理由 / 风险**：避免为单测引入与运行时不一致的 sqlite 方言（索引/类型差异）造成误判；DAO 行为以代码评审 + 集成验证保障。该取舍在 plan-report 中作为 testability 说明记录。

## D10 — migration 版本号与命名

- **决策**：新增文件 `<新递增时间戳>_add_process_managed_exception.go`，版本号需大于现存最新（`20250923114027`）。GormMode，`init()` 注册 `migrator.GetMigrator().AddMigration(&migrator.Migration{Mode: migrator.GormMode, Up, Down})`。
- **理由**：完全对标 `20250923114014_add_process.go` / `20250923114027_add_process_instance.go`；通过 `migrate create -n add_process_managed_exception` 生成骨架后填充（README）。
