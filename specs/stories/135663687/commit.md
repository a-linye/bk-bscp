# Commit 记录

## Commit Message

```
feat(dao): 新增进程托管异常记录表与读写 DAO

为父需求"GSE 托管信息检查"提供数据基础，新增独立的「托管异常记录」
数据载体，供检查侧写入/恢复、操作侧读取异常态。

- 新增 process_managed_exceptions 表与 ProcessManagedException 模型：
  业务字段 error_type/error_msg/handling_suggestion/status/checked_at（对标 gsekit），
  定位字段 tenant_id/biz_id/host_id/process_id/process_instance_id（冗余免 join）。
- error_type/status 为 string 枚举并提供 Validate()，风格对齐既有 ProcessStatus。
- DAO 能力：Create 追加写入（非覆盖，idGen 分配 ID）、ListByProcessInstanceID
  历史查询、GetLatestByProcessInstanceID + IsException 以最新记录判定异常态、
  UpdateStatus 恢复语义（刷新 reviser/updated_at，历史保留）。
- 建表 migration 含联合索引 idx_bizID_processInstanceID(biz_id, process_instance_id)；
  注册 gorm gen 并生成 internal/dal/gen；挂接 internal/dal/dao Set 接口。
- 表层枚举/结构以单包单测覆盖；DAO 行为类按测试基建现状取集成+评审验证 +
  编译期接口断言兜底（详见 plan-report A1）。

--story=135663687
```

## Commit Hash

<见提交后回填>

## 变更统计

| 指标 | 值 |
|------|-----|
| 总变更行数 | 2181 |
| 新增代码 | 2181 |
| 删除代码 | 0 |
| 逻辑代码 | 799（含 internal/dal/gen 生成物 456） |
| 测试代码 | 72 |
| 文档变更 | 1160（specs 设计/规范文档） |
| 变更文件数 | 27 |

## 成本汇总

### 总体

| 指标 | 值 |
|------|-----|
| 总耗时 | 0 s（本环境未触发成本采集 hook） |
| 总成本 | 0 credit |
| 总输入 tokens | 0 |
| 总输出 tokens | 0 |
| 总缓存 tokens | 0 |
| subagent 调用次数 | 9 |

> 说明：本环境无 cost-events.jsonl，PostToolUse 成本采集 hook 未触发，成本明细缺失，仅统计 subagent 调用次数。

## 时间

- 开始时间：2026-06-30T17:05:00+08:00
- 完成时间：2026-06-30T17:50:00+08:00
