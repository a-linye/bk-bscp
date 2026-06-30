# 数据模型：进程托管异常记录数据存储

**需求 ID**：短 ID 135663687 / 长 ID 1020451610135663687
**来源**：`spec.md`（FR-001~FR-012）、`req.md` 数据设计章节、`questions.md` Q1–Q6
**对标样板**：`pkg/dal/table/process.go`、`pkg/dal/table/process_instance.go`、`pkg/dal/table/config_template.go`

## 1. 命名确定（FR-001、req.md TR-002）

| 项 | 取值 | 说明 |
|----|------|------|
| 表名 | `process_managed_exceptions` | 表名常量 `ProcessManagedExceptionsTable` 登记于 `pkg/dal/table/table.go` |
| 模型名 | `ProcessManagedException` | 三段式：`ID + Attachment + Spec + Revision`（嵌入） |
| id_generators 资源名 | `process_managed_exceptions` | 与表名一致，`idGen.One/Batch` 据此分配 ID |

## 2. 模型结构（FR-001、FR-002、FR-003）

```go
// ProcessManagedException 托管异常记录：一条记录对应"某进程实例某次检查的异常结论"
type ProcessManagedException struct {
    ID         uint32                              `json:"id" gorm:"primaryKey"`
    Attachment *ProcessManagedExceptionAttachment  `json:"attachment" gorm:"embedded"`
    Spec       *ProcessManagedExceptionSpec        `json:"spec" gorm:"embedded"`
    Revision   *Revision                           `json:"revision" gorm:"embedded"`
}
```

### 2.1 Spec（业务字段，FR-002）

| 字段 | Go 类型 | 列名 | MySQL 类型 | 说明 |
|------|---------|------|-----------|------|
| ErrorType | `ProcessExceptionErrorType` | `error_type` | `varchar(64) not null` | 异常类型枚举（见 §4.1） |
| ErrorMsg | `string` | `error_msg` | `text` | 异常描述（含具体差异信息，长度不定，用 text） |
| HandlingSuggestion | `string` | `handling_suggestion` | `varchar(1024) not null;default:''` | 处理建议 |
| Status | `ProcessExceptionStatus` | `status` | `varchar(32) not null;default:'exception'` | 记录状态枚举（见 §4.2） |
| CheckedAt | `time.Time` | `checked_at` | `datetime;not null` | 检查时间，由检查侧写入时传入 |

### 2.2 Attachment（定位字段，冗余存储免 join，FR-003 / Q1）

| 字段 | Go 类型 | 列名 | MySQL 类型 | 说明 |
|------|---------|------|-----------|------|
| TenantID | `string` | `tenant_id` | `varchar(255) not null;default:'default'` | 由 `set_tenant_id` 回调自动注入；字段名必须为 `TenantID`（回调按此名 LookUpField） |
| BizID | `uint32` | `biz_id` | `bigint unsigned not null` | 业务 ID，联合索引列 1 |
| HostID | `uint32` | `host_id` | `bigint unsigned not null` | 主机 ID（process_instance 不含，取自 `ProcessAttachment`，Q1） |
| ProcessID | `uint32` | `process_id` | `bigint unsigned not null` | 关联 `processes` 表 ID |
| ProcessInstanceID | `uint32` | `process_instance_id` | `bigint unsigned not null` | 关联 `process_instances` 表 ID，联合索引列 2 |

### 2.3 Revision（嵌入 `table.Revision`，FR-009）

复用 `pkg/dal/table/table.go` 的 `Revision`：`creator` / `reviser` / `created_at` / `updated_at`。恢复操作刷新 `reviser` + `updated_at`。

## 3. 索引设计（FR-010、Q5）

| 索引名 | 列（优先级） | 类型 | 服务的查询 |
|--------|------------|------|-----------|
| `idx_bizID_processInstanceID` | `biz_id`(1), `process_instance_id`(2) | 普通索引 | FR-007 历史查询、FR-008 取最新记录判定（`WHERE biz_id=? AND process_instance_id=? ORDER BY id DESC`） |

> **biz_id 维度查询（FR-010"保留 biz_id 维度查询能力"）**：由上述联合索引的最左前缀 `biz_id` 覆盖，无需额外独立 `biz_id` 索引（避免冗余索引，符合 AGENTS.md"不引入不必要"）。
> **tenant_id（FR-010"作为多租户隔离前缀纳入查询条件"）**：由 `set_tenant_id` 回调自动追加 `tenant_id=?` 到 WHERE，不单独建索引；表数据先按 biz_id 收敛，租户过滤为附加条件。

## 4. 枚举定义

### 4.1 error_type（FR-004、Q2，对标 gsekit `ProcessCheckManager.ErrorType`）

```go
type ProcessExceptionErrorType string

const (
    ProcessExceptionParsingFailed       ProcessExceptionErrorType = "PARSING_FAILED"        // 解析失败
    ProcessExceptionAgentException      ProcessExceptionErrorType = "AGENT_EXCEPTION"       // agent 异常
    ProcessExceptionIllegalValueKey     ProcessExceptionErrorType = "ILLEGAL_VALUE_KEY"     // 非法 valuekey
    ProcessExceptionExpectationMismatch ProcessExceptionErrorType = "EXPECTATION_MISMATCH"  // 配置不符（已托管无信息/未托管有信息/属性差异）
    ProcessExceptionOther               ProcessExceptionErrorType = "OTHER"                 // 其他
)

func (t ProcessExceptionErrorType) Validate() error // 五值之外返回 error，风格对齐 ProcessStatus/AgentStatus.Validate()
```

### 4.2 status（FR-005、Q3）

```go
type ProcessExceptionStatus string

const (
    ProcessExceptionStatusException ProcessExceptionStatus = "exception"  // 异常
    ProcessExceptionStatusRecovered ProcessExceptionStatus = "recovered"  // 已恢复
)

func (s ProcessExceptionStatus) Validate() error // 两值之外返回 error
```

> 命名前缀统一用 `ProcessException`，避免与 `process_instance.go` 既有 `ProcessStatus` / `ProcessManagedStatus` 冲突。

## 5. DAO 契约（FR-006~FR-009）

接口 `ProcessManagedException`（`internal/dal/dao/process_managed_exception.go`），实现体 `processManagedExceptionDao{ genQ, idGen, auditDao }`，挂接到 `internal/dal/dao/dao.go` 的 `Set` 接口（FR-011）：

| 方法 | 签名要点 | 覆盖 | 语义 |
|------|---------|------|------|
| Create | `Create(kit, *table.ProcessManagedException) (uint32, error)` | FR-006 / AC-001 | 追加写入；ID 由 `idGen.One(kit, table.ProcessManagedExceptionsTable)` 分配；非覆盖 |
| ListByProcessInstanceID | `ListByProcessInstanceID(kit, bizID, processInstanceID uint32) ([]*table.ProcessManagedException, error)` | FR-007 / AC-002 | 返回该进程实例全部历史记录（`Order(id desc)`），租户隔离由回调生效 |
| GetLatestByProcessInstanceID | `GetLatestByProcessInstanceID(kit, bizID, processInstanceID uint32) (*table.ProcessManagedException, error)` | FR-008 | 按 `biz_id+process_instance_id` `Order(id desc).Take()`；无记录返回 `ErrRecordNotFound` |
| IsException | `IsException(kit, bizID, processInstanceID uint32) (bool, error)` | FR-008 / AC-004 / AC-T02 | 取最新一条；`ErrRecordNotFound`→`false,nil`；否则 `latest.Status==exception` |
| UpdateStatus | `UpdateStatus(kit, bizID, id uint32, status table.ProcessExceptionStatus) error` | FR-009 / AC-003 | 将目标记录 `status` 置为 `recovered` 并刷新 `reviser/updated_at`；历史明细保留 |

> 不实现 `AuditRes` 接口（`ResID/ResType`）：本表由后台巡检自动写入/恢复，非用户资源操作（Q4 / FR-005 范围外）。
> 写库失败：方法直接返回 error（调用方决定是否阻断），不在 DAO 层重试/吞错（边界场景"写库失败"）。

## 6. 关系

- `process_instance_id` → `process_instances.id`（逻辑关联，不建外键，沿用仓库惯例）。
- 冗余 `host_id` / `process_id` / `biz_id` / `tenant_id` 支撑免 join 定位与查询（Q1）。

## 7. 链路登记清单（FR-011）

1. `cmd/data-service/db-migration/migrations/<version>_add_process_managed_exception.go`：`AutoMigrate(&ProcessManagedException{})` + 向 `id_generators` 插入 `{Resource:"process_managed_exceptions"}`；Down 删表 + 删 generator 记录。
2. `pkg/dal/table/table.go`：新增 `ProcessManagedExceptionsTable Name = "process_managed_exceptions"`。
3. `scripts/gen/main.go`：`g.ApplyBasic(... , table.ProcessManagedException{})`，执行 `make gen` 生成 `internal/dal/gen/`。
4. `internal/dal/dao/dao.go`：`Set` 接口新增 `ProcessManagedException() ProcessManagedException`，`set` 结构体新增工厂方法。
