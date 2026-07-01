# Clarification Questions — Story 1020451610135663687

## Q1 [resolved_by_doc] — 来源：subagent(speckit.specify)
**问题**：「托管异常记录」的定位字段以哪些为准？process_instance 表自身是否已包含 host_id？
**影响**：决定异常记录表结构的 Attachment 字段；非阻塞。
**建议候选**：
- A. 冗余存储 tenant_id + biz_id + host_id + process_id + process_instance_id（推荐：定位与查询免 join）
- B. 仅存 process_instance_id，其余 join 查询
**提出方**：subagent(speckit.specify) / attempt=1 / round=1 / ts=2026-06-30T17:06:00+08:00
**答复**：采用 A。`pkg/dal/table/process_instance.go` 的 `ProcessInstanceAttachment` 仅含 tenant_id/biz_id/process_id/cc_process_id，**不含 host_id**；host_id 位于 `pkg/dal/table/process.go` 的 `ProcessAttachment`。异常记录需支撑"按业务/进程实例查询"且检查侧/操作侧均需快速定位，故在异常记录 Attachment 冗余存储 tenant_id、biz_id、host_id、process_id、process_instance_id，避免查询 join。租户 tenant_id 由 `internal/dal/dao/set_tenant_id.go` 回调自动处理。
**答复方**：subagent(自答) / ts=2026-06-30T17:06:00+08:00
**文档来源**：pkg/dal/table/process_instance.go, pkg/dal/table/process.go

## Q2 [resolved_by_doc] — 来源：subagent(speckit.specify)
**问题**：error_type 异常类型枚举取值如何确定，是否对标 gsekit？
**影响**：决定 error_type 枚举定义；非阻塞。
**建议候选**：
- A. 直接对标 gsekit `ErrorType` 五值（推荐）
**提出方**：subagent(speckit.specify) / attempt=1 / round=1 / ts=2026-06-30T17:06:00+08:00
**答复**：对标 gsekit `ProcessCheckManager.ErrorType`：PARSING_FAILED（解析失败）/ AGENT_EXCEPTION（agent 异常）/ ILLEGAL_VALUE_KEY（非法 valuekey）/ EXPECTATION_MISMATCH（配置不符，涵盖"已托管无信息""未托管有信息""属性差异"）/ OTHER（其他）。定义为 string 枚举并提供 Validate()，与仓库 `ProcessStatus`/`AgentStatus` 风格一致。
**答复方**：subagent(自答) / ts=2026-06-30T17:06:00+08:00
**文档来源**：bk-process-config-manager/apps/gsekit/process/handlers/check_process.py

## Q3 [resolved_by_doc] — 来源：subagent(speckit.specify)
**问题**：记录状态枚举取值，以及"当前是否处于异常态"如何用"最近一次记录状态"实现？
**影响**：决定 status 枚举与判定查询逻辑；非阻塞。
**建议候选**：
- A. status ∈ {exception, recovered}，按 process_instance_id 取最新一条记录判定（推荐）
**提出方**：subagent(speckit.specify) / attempt=1 / round=1 / ts=2026-06-30T17:06:00+08:00
**答复**：状态枚举 status ∈ { exception（异常）, recovered（已恢复）}，string 枚举 + Validate()。"当前是否异常"判定查询：按 process_instance_id（+ biz_id/tenant_id）取最新一条记录（按 id 或 created_at 降序取首条），其 status==exception 即异常，否则（无记录或最新为 recovered）非异常。与 req.md 边界条件"以最近一次记录状态判断"一致。
**答复方**：subagent(自答) / ts=2026-06-30T17:06:00+08:00
**文档来源**：specs/stories/135663687/req.md, docs/reqs/GSE托管信息检查.md

## Q4 [resolved_by_doc] — 来源：subagent(speckit.specify)
**问题**：异常记录表是否需要接入审计（audit）能力（实现 ResID/ResType 并写 audits 表）？
**影响**：决定模型是否实现 AuditRes 接口；非阻塞。
**建议候选**：
- A. 不接入 audit（推荐）
- B. 接入 audit
**提出方**：subagent(speckit.specify) / attempt=1 / round=1 / ts=2026-06-30T17:06:00+08:00
**答复**：不接入 audit。异常记录由后台巡检（检查侧）自动写入、由检查通过自动恢复，并非用户对资源的操作变更；父需求与本需求均未提出审计诉求。依据 AGENTS.md"不引入不必要的抽象/配置项/兼容层"，本期不实现 AuditRes 接口。
**答复方**：subagent(自答) / ts=2026-06-30T17:06:00+08:00
**文档来源**：AGENTS.md, specs/stories/135663687/req.md, docs/reqs/GSE托管信息检查.md

## Q5 [resolved_by_doc] — 来源：subagent(speckit.specify)
**问题**：大表查询的索引如何设计？
**影响**：决定 migration 建表索引；非阻塞。
**建议候选**：
- A. (biz_id, process_instance_id) 联合索引 + biz_id 索引（推荐）
**提出方**：subagent(speckit.specify) / attempt=1 / round=1 / ts=2026-06-30T17:06:00+08:00
**答复**：按 req.md 性能需求"按业务/进程实例查询需走索引"，建立联合索引覆盖 (biz_id, process_instance_id)（同时服务"按进程实例取最新记录"判定查询，可叠加 id 降序），并保留 biz_id 维度查询能力；tenant_id 作为多租户隔离前缀按仓库惯例纳入查询条件。具体索引在 migration 建表脚本中落地。
**答复方**：subagent(自答) / ts=2026-06-30T17:06:00+08:00
**文档来源**：specs/stories/135663687/req.md

## Q6 [resolved_by_doc] — 来源：subagent(speckit.specify)
**问题**：历史明细如何保留（追加 vs 覆盖）？恢复时是更新原记录还是新增记录？
**影响**：决定写入与状态更新 DAO 的语义；非阻塞。
**建议候选**：
- A. 写入追加新行；恢复时更新对应异常记录 status=recovered（推荐）
**提出方**：subagent(speckit.specify) / attempt=1 / round=1 / ts=2026-06-30T17:06:00+08:00
**答复**：异常写入以追加方式新增记录（非覆盖），保留同一进程实例的历史多条；状态更新（恢复）将目标异常记录 status 置为 recovered 并更新 updated_at（reviser），历史明细仍可追溯。符合 AC-002（历史非覆盖）与 AC-003（恢复后判定查询返回否）。
**答复方**：subagent(自答) / ts=2026-06-30T17:06:00+08:00
**文档来源**：specs/stories/135663687/req.md
