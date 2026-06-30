# Validate-Security Report — Story 1020451610135663687

## Verdict
LGTM

## Checked artifacts
- cmd/data-service/db-migration/migrations/20260630173000_add_process_managed_exception.go
- pkg/dal/table/process_managed_exception.go
- pkg/dal/table/table.go（ProcessManagedExceptionsTable 常量）
- internal/dal/dao/process_managed_exception.go
- internal/dal/dao/dao.go（Set 接口挂载）

## Reference baselines
- .cursor/skills/bk-security-redlines/SKILL.md（三大红线：输入校验 / 鉴权 / 敏感数据）
- specs/stories/135663687/spec.md（FR-003、FR-004、FR-005、FR-010、FR-012）
- specs/stories/135663687/data-model.md（DAO 契约、租户隔离、索引）
- internal/dal/dao/set_tenant_id.go（多租户 GORM 回调）
- AGENTS.md

## 维度结论

### 1. 输入校验（红线 1）
- DAO 全部查询/更新使用 gen 类型安全 Where：`m.BizID.Eq(bizID)`、`m.ProcessInstanceID.Eq(...)`、`m.ID.Eq(id)`，参数化绑定，**无裸 SQL 字符串拼接**。
- `UpdateStatus` 使用 `Updates(map[string]any{...})`，键为常量列名、值为强类型枚举/`kit.User`/`time.Now()`，不可注入。
- migration `Down` 的 `tx.Where("resource IN ?", resources)` 为参数化占位符，`resources` 为代码内常量切片，安全。
- 枚举 `ProcessExceptionErrorType` / `ProcessExceptionStatus` 均提供白名单式 `Validate()`（FR-004 / FR-005）。

### 2. 鉴权 / 越权（红线 2）
- 表名未列入 `excludedTables`，模型 `TenantID` 字段名与回调 `LookUpField("TenantID")` 一致，写入经 `beforeAnyOp` 自动注入 tenant_id、查询/更新经 `beforeQuery` 自动追加 `tenant_id` 过滤，**租户隔离生效**（AC-T01 / SC-006）。
- 所有读取/更新方法均带 `biz_id` 维度（`UpdateStatus` 同时带 `id` + `biz_id` + 自动 tenant），无横向越权读/改他业务或他租户记录的路径。
- 本表由后台巡检自动写入/恢复，非用户资源操作，不接审计符合 spec 范围外约定（data-model §5）。

### 3. 敏感数据（红线 3）
- 无硬编码密钥/凭证/token。
- DAO 层不打印 error_msg / 业务字段日志，无敏感数据回显或落日志风险。
- 模型未强制任何 PII 字段，FR-012 在数据层不被违反（error_msg 内容由检查侧写入侧约束）。

### 4. 常见风险
- SQL 注入：无（gen 参数化）。路径穿越 / 不安全反序列化 / SSRF：本变更纯数据层 CRUD，均不涉及。

## Findings

### A1
- **类别**：Security（输入校验，纵深防御）
- **严重性**：LOW
- **位置**：internal/dal/dao/process_managed_exception.go:48-65（Create）
- **总结**：`Create` 写入前未调用 `m.Spec.ErrorType.Validate()` / `m.Spec.Status.Validate()`，依赖调用方传入合法枚举。
- **根因**：code-self
- **修改建议**：枚举值来自内部检查侧（非外部不可信输入）且为强类型、参数化落库，注入风险为零；当前为可接受的纵深防御缺口。后续检查侧子需求接入写入时，建议在 Create 入口或调用方补一次 `Validate()` 做防御性校验。不构成本期 [必须] 项。

> 无 CRITICAL / HIGH [必须] 项；仅 1 项 LOW 级建议。依据 validate 段判定规则，Verdict 为 LGTM。
