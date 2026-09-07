# 进程启动优先级：gsekit vs bscp 调研结论

> 背景：bscp 承接 gsekit 进程操作能力后，用户启动失败，怀疑与启动顺序/优先级有关。  
> 目的：厘清「优先级顺序」是 GSE 能力还是 gsekit 编排能力，并对比 bscp 现状。  
> 结论基于源码核对，非推测。

## 1. 核心结论

**优先级顺序不是 GSE 的能力，而是 gsekit 后台按 CMDB 进程字段 `priority`（启动优先级）自行编排的。**

| 层级 | 是否感知 priority | 说明 |
|------|-------------------|------|
| CMDB | 有字段 | 进程属性 `priority`，注释为「启动优先级」 |
| gsekit | **负责编排** | 读 CMDB `priority`，用 Bamboo Pipeline 按优先级分批串行 |
| GSE | **不感知** | `operate_proc_multi` 请求体无 priority 字段，只按提交的进程列表执行 |
| bscp | **未实现** | 本地 `ProcessInfo` 无 priority；任务按实例拆开并发下发，无优先级门控 |

因此：若业务依赖「低 priority 先起、高 priority 后起」，gsekit 能保证，当前 bscp **不能保证**。

---

## 2. priority 字段来源：CMDB

CMDB 进程对象带有 `priority` 字段（启动优先级）。bscp 的 CMDB 客户端类型定义中已有该字段：

```go
// bk-bscp/internal/components/bkcmdb/type.go
Priority int `json:"priority"` // 启动优先级
```

gsekit 历史迁移也将旧字段 `Seq` 映射为 `priority`（描述：启动优先级），见：

- `bk-process-config-manager/apps/gsekit/migrate/handlers.py`

gsekit 创建 JobTask 时，从 CMDB 进程关联信息写入 `extra_data["process_info"]["process"]["priority"]`，空值默认 `0`：

- `apps/gsekit/adapters/base/pipeline_managers/base.py`（`generate_to_be_created_data`）

---

## 3. gsekit 如何用 priority 编排（不是 GSE 排序）

### 3.1 Pipeline 分批串行

入口：`ProcessPipelineManager.create_pipeline`

路径：`apps/gsekit/adapters/base/pipeline_managers/process.py`

编排逻辑：

1. 先按任务粒度（业务 / 集群 / 模块 / 主机）分组（`process_task_aggregate_info`）
2. 组内按 `priority` 排序，再 `groupby(priority)`
3. **同 priority**：打成一批 Bulk Activity，内部并行
4. **不同 priority**：Activity 串联，前一批完成后再执行下一批

操作方向：

| 操作 | 顺序 |
|------|------|
| 启动（START）等 | priority **升序**（小的先执行） |
| 停止（STOP）/ 重启（RESTART） | priority **降序**（大的先执行） |
| 托管 / 取消托管 | 强制把 priority 置为 0，不区分优先级 |

示意图（启动）：

```text
Start
  → [priority=1 并行批量操作 + check]
  → [priority=2 并行批量操作 + check]
  → ...
  → End
```

### 3.2 失败时的优先级门控

某 priority 阶段失败时，`activity_failed_handler` 会把**后续不应再执行**的 PENDING 任务直接置失败：

- 路径：`apps/gsekit/pipeline_plugins/signals.py`
- 正向操作：失败后，更高 priority 不再执行
- 逆向操作（停/重启）：失败后，更低 priority 不再执行

错误文案明确写的是「优先级等于 [x] 的进程操作已失败，…不会被继续执行」，属于 **gsekit 业务编排**，不是 GSE 返回的排序错误。

### 3.3 与「实例合并」的关系（易混淆点）

gsekit 还有一层「合并」，但**不是 priority 机制**：

- 在 Bulk GSE 操作原子里，按 `local_inst_name = {bk_process_name}_{local_inst_id}` 分组
- 同名实例跨主机合并进同一请求的 `hosts`，再调 `operate_proc_multi`
- 目的：减少 GSE 调用次数

相关代码：

- `apps/gsekit/pipeline_plugins/components/collections/gse.py`（`BulkGseOperateProcessService._execute`）
- 状态查询同类逻辑：`apps/gsekit/process/handlers/process.py`（`get_proc_inst_status_infos` 注释亦写明）

**合并 ≠ 优先级。** 优先级由 Pipeline 阶段串行保证；合并只发生在同一批（通常同 priority）内部的 GSE 请求构造。

---

## 4. GSE 侧：无启动优先级能力

bscp 封装的 GSE 进程操作请求结构（与 gsekit 提交结构一致）大致为：

- `meta`（namespace / name）
- `agent_id_list` / `hosts`
- `op_type`
- `spec`（identity / control / monitor_policy）

见：`bk-bscp/internal/components/gse/type.go` → `ProcessOperate`

**整条请求链没有 `priority` 字段。** GSE 收到的是「对哪些 Agent、对哪个进程、做什么操作」；谁先谁后由**调用方分几次、按什么顺序调用**决定。

因此：

- GSE：执行引擎（按请求操作进程）
- gsekit：调度编排（按 CMDB priority 决定调用顺序）
- CMDB：优先级数据源

---

## 5. bscp 现状对比

### 5.1 任务下发：一实例一任务，立即 Dispatch

入口：`cmd/data-service/service/process.go` → `OperateProcess` → `dispatchProcessTasks`

- 每个进程实例单独 `buildProcessTask` + `taskManager.Dispatch`
- **无**按 priority 排序
- **无**「同 priority 一批、跨 priority 串行等待」
- 同批次任务实质并发执行（受任务框架并发度限制，但无业务优先级门控）

### 5.2 执行端：单 Agent 单次 GSE 调用

入口：`internal/task/executor/process/process.go` → `Operate`

- `AgentID: []string{proc.AgentID}`（单主机）
- `ProcOperateReq` 长度为 1
- 不做跨主机合并，也不读 priority

### 5.3 本地模型未落地 priority

`pkg/dal/table/process.go` 的 `ProcessInfo` 仅含命令/路径/超时等字段，**没有 `priority`**。  
CMDB 客户端虽能解析 `priority`，进程操作链路未消费该字段。

---

## 6. 对比总表

| 维度 | gsekit | bscp（当前） | GSE |
|------|--------|--------------|-----|
| priority 数据源 | CMDB `process.priority` | CMDB 可取，但未用于操作编排 | 无 |
| 启动顺序 | Pipeline 按 priority 升序分批串行 | 无顺序保证 | 不负责 |
| 停止/重启顺序 | priority 降序 | 无顺序保证 | 不负责 |
| 同 priority 内 | 批量并行 + 可跨主机合并 hosts | 各实例独立并发任务 | 执行提交的列表 |
| 前序失败后 | 信号门控后续 priority | 仅单任务失败，无跨任务 priority 级联 | 无 |
| 跨主机同名实例 | 可合并到同一 GSE 请求 | 不合并 | 按请求执行 |

---

## 7. 对「用户启动失败 / 顺序问题」的含义

若故障现象符合「依赖进程未就绪时，依赖方已启动失败」，更合理的根因是：

1. **bscp 未实现 CMDB priority 编排**（主因）  
2. 而非「拆开下发导致合并丢失」本身  

「拆开下发」主要影响：

- GSE 调用次数 / 批量性
- 同名跨主机是否同请求

**不影响**跨不同进程别名之间的启动依赖顺序；那种顺序只能靠上层像 gsekit 一样按 priority 串行。

---

## 8. 若要对齐 gsekit：能力缺口（调研级，非方案定稿）

最小对齐项：

1. **同步并持久化** CMDB `priority`（进程维度）
2. **OperateProcess 编排**：同拓扑范围内按 priority 分批；启动升序、停止/重启降序
3. **批次门控**：前一 priority 批次全部成功（或策略允许）后再派发下一批；失败时级联取消后续
4. （可选）同批次内按 `alias/local_inst` 合并多 Agent，对齐 gsekit 调用形态——与顺序无关，属性能/行为一致性

---

## 9. 关键源码索引

| 主题 | 路径 |
|------|------|
| gsekit priority 编排 | `bk-process-config-manager/apps/gsekit/adapters/base/pipeline_managers/process.py` |
| gsekit JobTask 写入 priority | `.../adapters/base/pipeline_managers/base.py` |
| gsekit 失败级联 | `.../pipeline_plugins/signals.py` |
| gsekit GSE 批量合并 hosts | `.../pipeline_plugins/components/collections/gse.py` |
| gsekit 状态查询合并说明 | `.../process/handlers/process.py`（`get_proc_inst_status_infos`） |
| CMDB priority 类型 | `bk-bscp/internal/components/bkcmdb/type.go` |
| GSE 请求结构（无 priority） | `bk-bscp/internal/components/gse/type.go` |
| bscp 下发任务 | `bk-bscp/cmd/data-service/service/process.go` |
| bscp 执行操作 | `bk-bscp/internal/task/executor/process/process.go` |
| bscp ProcessInfo | `bk-bscp/pkg/dal/table/process.go` |

---

## 10. 一句话回答

> **优先级顺序是 gsekit 后台读取 CMDB 的 `priority` 后，用 Pipeline 分批串行实现的调度能力；不是 GSE 接口能力。当前 bscp 未承接该编排，因此 CMDB 上配置的启动优先级对 bscp 进程操作基本没有意义。**
