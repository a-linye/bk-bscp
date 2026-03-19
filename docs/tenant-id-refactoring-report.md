# BSCP 租户 ID (TenantID) 改造报告

## 一、背景与问题

BSCP 引入多租户架构后，通过 GORM 回调（`beforeQuery`/`beforeAnyOp`）在数据库查询中自动注入 `tenant_id` 过滤条件，实现数据隔离。但在实际运行中发现多处场景下 `tenant_id` 缺失或传播不正确，导致：

1. **非 default 租户的数据被过滤掉**：当 `kit.TenantID` 为空时，GORM 回调注入 `tenant_id IN ('default', '')`，导致其他租户的数据查不到
2. **feed-server 请求链路中 TenantID 始终为空**：feed-server 的客户端（sidecar）通过 Bearer Token 认证，不携带 `X-Bk-Tenant-Id` 头，需要从 `biz_id` 反查租户
3. **cache-service 事件消费缺少租户上下文**：事件循环使用空 Kit，消费事件后的 DAO 操作缺少租户过滤
4. **Task 执行器缺少租户 ID**：异步任务的 Payload 中没有传递 `TenantID`，执行时直接用 `kit.New()` 跳过租户过滤

## 二、核心机制

### 2.1 GORM 回调自动注入

GORM 回调 `beforeQuery` 从 `db.Statement.Context` 中提取 `kit.Kit`，根据 `kt.TenantID` 的值决定过滤策略：

```go
// internal/dal/dao/set_tenant_id.go
kt := kit.FromGrpcContext(db.Statement.Context)
if kt.TenantID == "" {
    // 兼容旧数据（空字符串）和新数据（default）
    tenantExpr = clause.IN{Column: "tenant_id", Values: []interface{}{"default", ""}}
} else {
    tenantExpr = clause.Eq{Column: "tenant_id", Value: kt.TenantID}
}
```

`kit.Kit.Ctx` 必须通过 `InternalRpcCtx()` 或 `RpcCtx()` 将 `TenantID` 序列化到 gRPC metadata 中，`FromGrpcContext()` 才能正确读取。

### 2.2 关键工具函数

| 函数 | 用途 |
|---|---|
| `kit.NewWithTenant(tenantID)` | 创建携带指定 TenantID 的 Kit，自动调用 `InternalRpcCtx()` |
| `kit.WithSkipTenantFilter()` | 克隆 Kit 并设置 `SkipTenantFilterKey`，跳过租户过滤 |
| `EnsureTenantID(kt, bizID)` | 通过 biz_id 反查租户并设置到 kt.TenantID（两级缓存） |

### 2.3 `biz_id` 全局唯一性约束

`biz_id` 在整个系统中是全局唯一递增的，不同租户不会有相同的 `biz_id`。这意味着 `EnsureTenantID(bizID)` 可以安全地通过 `biz_id` 反查唯一的租户。

## 三、改造范围

改造涉及 **3 个文件（未提交）+ 45 个文件（已提交，commit 330a850d）**，分为以下几个维度。

### 3.1 feed-server：租户 ID 解析与传播

#### 未提交改动（3 个文件）

| 文件 | 改动 | 说明 |
|---|---|---|
| `cmd/feed-server/service/interceptor.go` | `authorize()` 函数增加 `EnsureTenantID` 调用 | 凭证认证前先通过 biz_id 解析 tenant_id，否则凭证查询会因 tenant_id 过滤而失败 |
| `cmd/feed-server/service/service.go` | `DownloadFile()` 函数增加 `EnsureTenantID` 调用 | 文件下载 HTTP 路径不经过 gRPC 拦截器，需独立解析租户 |
| `internal/iam/auth/middleware.go` | `initKitWithDevEnv()` 支持从 `X-Bk-Tenant-Id` Header 读取 tenant_id | Dev 环境支持手动注入租户，方便测试 |

#### 已提交改动

| 文件 | 改动 |
|---|---|
| `cmd/feed-server/service/rpc_sidecar.go` | `Handshake`、`PullAppFileMeta`、`PullKvMeta`、`GetKvValue` 等 4 个 RPC handler 中增加 `EnsureTenantID` 调用 |
| `cmd/feed-server/bll/lcache/app.go` | 新增 `EnsureTenantID` 方法：通过 biz_id 查询 app 表（跳过租户过滤）获取 tenant_id 并缓存到 Redis 和本地 gcache |
| `cmd/feed-server/bll/eventc/app_event.go` | `AddSidecar` 中增加 `EnsureTenantID` 调用 |
| `cmd/feed-server/bll/eventc/scheduler.go` | 调度器事件处理增加租户上下文 |

### 3.2 cache-service：事件消费租户上下文

| 文件 | 改动 | 说明 |
|---|---|---|
| `cache/event/consumer.go` | 事件消费时从 `event.Attachment.BizID` 解析 `TenantID`，传入后续 DAO 操作 | 解决事件消费缺少租户的问题 |
| `cache/event/loop_watch.go` | 事件循环入口使用 `WithSkipTenantFilter()` | 事件是全局资源，需跨租户查询 |
| `cache/event/publish.go` | 发布消费时携带正确的租户上下文 | — |
| `cache/event/purge.go` | 清理过期事件跨租户 | — |
| `cache/client/client.go` | 新增 `ResolveTenantID` 方法供事件消费调用 | — |
| `cache/client/app.go` | `GetTenantIDByBiz` 查询并缓存租户映射 | — |
| `service/feed.go` | `GetTenantIDByBiz` RPC handler | — |

### 3.3 data-service：定时任务与后台作业

| 文件 | 改动 |
|---|---|
| `crontab/cmdb_resource_watcher.go` | CMDB 进程事件中提取 `TenantID`，传入 Task |
| `crontab/cleanup_biz_host.go` | 使用 `NewWithTenant` 或 `WithSkipTenantFilter` |
| `crontab/sync_biz_host.go` | 同上 |
| `crontab/watch_biz_host_relation.go` | 同上 |
| `service/repo_syncer.go` | 仓库同步跨租户，使用 `WithSkipTenantFilter` |
| `service/config_instance.go` | Task 创建传递 TenantID |
| `service/process.go` | Task 创建传递 TenantID |

### 3.4 Task 执行器：异步任务租户传播

| 模块 | 改动 |
|---|---|
| `pkg/kit/kit.go` | 新增 `NewWithTenant(tenantID)` 工具函数 |
| `internal/task/builder/` | 所有 Task Builder 的 Options 结构增加 `TenantID` 字段（common、config_check、config_generate、config_push、process、gse 等） |
| `internal/task/executor/` | 所有 Executor 的 Payload 增加 `TenantID`，用 `kit.NewWithTenant(payload.TenantID)` 替代 `kit.New().WithSkipTenantFilter()` |
| `internal/task/step/` | 所有 Step 函数签名增加 `TenantID` 参数传递 |
| `internal/task/executor/common/common.go` | 回调通知增加 `TenantID` |

### 3.5 DAO 层与策略查询

| 文件 | 改动 |
|---|---|
| `internal/dal/dao/set_tenant_id.go` | 增加 `excludedTables`（clients、client_events），增加防重复注入检查 |
| `internal/dal/dao/app.go` | `GetOneAppByBiz` 增加 `tenant_id <> ''` 条件，防止返回空 tenant 的脏数据 |
| `internal/dal/dao/strategy.go` | 策略查询使用 `WithSkipTenantFilter` |
| `internal/dal/dao/template_space.go` | `GetAllBizs` 使用 `WithSkipTenantFilter` |
| `pkg/criteria/constant/key.go` | 新增 `SkipTenantFilterKey` 常量 |

### 3.6 其他

| 文件 | 改动 |
|---|---|
| `internal/processor/cmdb/sync_cmdb.go` | CMDB 同步增加租户上下文 |

## 四、关键设计决策

### 4.1 `WithSkipTenantFilter()` 的使用原则

仅在以下系统级场景使用，不应在业务查询中使用：

| 场景 | 原因 |
|---|---|
| 事件循环 `ListEventsMeta` | 事件是全局资源，需跨租户查询 |
| `GetOneAppByBiz` (反查租户) | 鸡蛋问题：需要先查 app 才能知道租户 |
| `GetAllBizs` (获取所有业务) | 系统级聚合操作 |
| `Purge` (清理过期事件) | 清理全局事件 |
| `GetCurrentCursorReminder` | 全局游标 |
| `repo_syncer.syncAll` | bkrepo 全量同步 |
| `RecordCursor` (记录事件游标) | 全局游标写入 |
| `GetStrategyByIDs` (事件消费内) | 消费事件时策略可能属于任意租户 |

### 4.2 `clients` 和 `client_events` 表排除

这两个表被加入 `excludedTables`，GORM 回调不会为它们注入 `tenant_id` 条件。原因是客户端上报的 metrics 数据在当前架构下不区分租户。

### 4.3 `EnsureTenantID` 两级缓存

```
请求 → 本地 gcache (进程内, ms级) → Redis DB=1 (跨进程, ms级) → DB 查询 (首次)
```

缓存 key 格式：`{biz_id}bscp:tenant-id:tenant-id`

## 五、测试范围说明

### 5.1 测试环境

- 单实例 Dev 环境，手动构造多租户数据
- tenant_a → biz_id=2，tenant_b → biz_id=3
- 数据构成：

| 租户 | App (file) | App (kv) | 配置项 | KV | 版本 | 凭证 | 模板空间 | 分组 |
|---|---|---|---|---|---|---|---|---|
| tenant_a | 2 | 1 | 2 | 5 | 4 | 1 | 1 | 1 |
| tenant_b | 2 | 1 | 1 | 3 | 2 | 1 | 1 | 1 |

### 5.2 测试矩阵

#### 5.2.1 API Server → Config Server → Data Service 链路（读操作隔离）

| # | 测试项 | 方法 | 预期 | 结果 |
|---|---|---|---|---|
| 1 | 创建 App (file/kv) | POST create app | 写入正确 tenant_id | ✅ |
| 2 | 列出 App | POST list app | 仅返回本租户数据 | ✅ |
| 3 | 跨租户列出 App | tenant_b 查 biz=2 | count=0 | ✅ |
| 4 | 创建配置项 | BatchUpsert | 写入正确 tenant_id | ✅ |
| 5 | 上传文件内容 | PUT upload | 成功上传 | ✅ |
| 6 | 创建版本 | POST release | 写入正确 tenant_id | ✅ |
| 7 | 发布版本 | POST publish (all=true) | 成功发布 | ✅ |
| 8 | 创建 KV | POST kv | 写入正确 tenant_id | ✅ |
| 9 | 列出 KV | POST list kv | 仅本租户 | ✅ |
| 10 | 跨租户 KV | tenant_b 查 tenant_a 的 kv app | APP_NOT_EXISTS | ✅ |
| 11 | 创建凭证 | POST credential | 写入正确 tenant_id | ✅ |
| 12 | 配置凭证作用范围 | PUT scope | 成功 | ✅ |
| 13 | 创建模板空间 | POST template_spaces | 写入正确 tenant_id | ✅ |
| 14 | 跨租户模板空间 | tenant_b 查 biz=2 | count=0 | ✅ |
| 15 | 创建分组 | POST groups | 写入正确 tenant_id | ✅ |
| 16 | 跨租户分组 | tenant_b 查 biz=2 | count=0 | ✅ |

#### 5.2.2 跨租户写操作隔离

| # | 测试项 | 预期 | 结果 |
|---|---|---|---|
| 17 | tenant_b 更新 tenant_a 的 App | record not found（更新被阻止） | ✅ |
| 18 | tenant_b 删除 tenant_a 的 App | record not found（删除被阻止） | ✅ |
| 19 | tenant_b 更新 tenant_a 的 KV | APP_NOT_EXISTS | ✅ |
| 20 | tenant_b 删除 tenant_a 的分组 | record not found | ✅ |
| 21 | 操作后确认原数据完整 | 数据未变 | ✅ |

#### 5.2.3 Feed Server 链路

| # | 测试项 | 链路 | 预期 | 结果 |
|---|---|---|---|---|
| 22 | KV 元数据拉取 | feed→cache→data | 返回正确的 KV 列表 | ✅ |
| 23 | KV 值拉取 | feed→cache→data | 返回正确值且隔离 | ✅ |
| 24 | 文件配置下载 | feed→bkrepo | 返回正确文件内容 | ✅ |
| 25 | 跨租户凭证拒绝 (KV) | tenant_a token 访问 biz=3 | 401 Unauthenticated | ✅ |
| 26 | 跨租户凭证拒绝 (文件) | tenant_a token 下载 biz=3 文件 | 401 Unauthenticated | ✅ |
| 27 | 凭证-租户绑定验证 | feed→cache GetCredential SQL | tenant_id 过滤正确 | ✅ |

#### 5.2.4 Cache Service 链路

| # | 测试项 | 说明 | 结果 |
|---|---|---|---|
| 28 | 事件消费 | 新发布后 cache-service 正确消费事件 | ✅ |
| 29 | 缓存刷新 | 发布后 feed-server 立即拉取到新数据 | ✅ |
| 30 | RPC TenantID 传播 | 所有 cache RPC 日志显示正确 TenantID | ✅ |
| 31 | Redis 缓存正确性 | biz=2→tenant_a, biz=3→tenant_b | ✅ |

#### 5.2.5 数据库层验证

| # | 测试项 | 结果 |
|---|---|---|
| 32 | applications 表 tenant_id 正确 | ✅ |
| 33 | config_items 表 tenant_id 正确 | ✅ |
| 34 | kvs 表 tenant_id 正确 | ✅ |
| 35 | releases 表 tenant_id 正确 | ✅ |
| 36 | strategies 表 tenant_id 正确 | ✅ |
| 37 | credentials 表 tenant_id 正确 | ✅ |
| 38 | credential_scopes 表 tenant_id 正确 | ✅ |
| 39 | events 表 tenant_id 正确 | ✅ |
| 40 | data-service INSERT SQL 携带 tenant_id | ✅ |
| 41 | data-service SELECT SQL 携带 tenant_id 过滤 | ✅ |
| 42 | cache-service SELECT SQL 携带 tenant_id 过滤 | ✅ |

### 5.3 未覆盖范围

| 项目 | 原因 | 风险评估 |
|---|---|---|
| Hook（前置/后置脚本） | 当前环境无 Hook 配置 | **低**：Hook 走标准 DAO 链路，GORM 回调自动注入 |
| 模板文件引用 | 未构造模板配置项数据 | **低**：模板引用走标准 release 发布链路 |
| gRPC 直连 feed-server | 需 sidecar 客户端 | **中**：gRPC 路径已有 `EnsureTenantID` 但未直接验证 |
| Task 执行器实际触发 | 需 CMDB 联动等环境 | **中**：代码已改造，但未在运行时触发验证 |
| 审计日志 (audit) | 审计模块有 Username 校验问题 | **低**：与租户隔离无关 |
| 多实例 / 集群环境 | 本次为单实例测试 | 需后续集成环境覆盖 |

## 六、变更文件清单

### 已提交 (commit 330a850d)

```
cmd/cache-service/service/cache/client/app.go
cmd/cache-service/service/cache/client/client.go
cmd/cache-service/service/cache/event/consumer.go
cmd/cache-service/service/cache/event/loop_watch.go
cmd/cache-service/service/cache/event/publish.go
cmd/cache-service/service/cache/event/purge.go
cmd/cache-service/service/feed.go
cmd/data-service/service/config_instance.go
cmd/data-service/service/crontab/cleanup_biz_host.go
cmd/data-service/service/crontab/cmdb_resource_watcher.go
cmd/data-service/service/crontab/sync_biz_host.go
cmd/data-service/service/crontab/watch_biz_host_relation.go
cmd/data-service/service/process.go
cmd/data-service/service/repo_syncer.go
cmd/feed-server/bll/eventc/app_event.go
cmd/feed-server/bll/eventc/scheduler.go
cmd/feed-server/bll/lcache/app.go
cmd/feed-server/service/rpc_sidecar.go
internal/dal/dao/app.go
internal/dal/dao/set_tenant_id.go
internal/dal/dao/strategy.go
internal/dal/dao/template_space.go
internal/processor/cmdb/sync_cmdb.go
internal/task/builder/common/common.go
internal/task/builder/config/config_check.go
internal/task/builder/config/config_generate.go
internal/task/builder/config/config_push.go
internal/task/builder/gse/process_state_sync.go
internal/task/builder/process/process.go
internal/task/builder/process/update_register.go
internal/task/executor/common/common.go
internal/task/executor/config/config_check.go
internal/task/executor/config/config_generate.go
internal/task/executor/config/config_push.go
internal/task/executor/gse/process_state_sync.go
internal/task/executor/process/process.go
internal/task/executor/process/update_register.go
internal/task/step/config/config_check.go
internal/task/step/config/config_generate.go
internal/task/step/config/config_push.go
internal/task/step/gse/sync_gse.go
internal/task/step/process/process.go
internal/task/step/process/update_register.go
pkg/criteria/constant/key.go
pkg/kit/kit.go
```

### 未提交

```
cmd/feed-server/service/interceptor.go    (authorize 增加 EnsureTenantID)
cmd/feed-server/service/service.go        (DownloadFile 增加 EnsureTenantID)
internal/iam/auth/middleware.go            (Dev 环境支持 tenant_id header)
```

## 七、注意事项

1. **数据迁移**：存量数据的 `tenant_id` 为空字符串，需执行 SQL 迁移为 `'default'` 或实际租户值。涉及表包括：applications、config_items、kvs、releases、strategies、credentials、credential_scopes、groups、template_spaces 等
2. **Redis 缓存清理**：数据迁移后必须清理 Redis DB=1 中的 `{biz}bscp:tenant-id:tenant-id` 等缓存键，否则会使用旧的缓存值
3. **`excludedTables` 维护**：`clients` 和 `client_events` 表不参与租户过滤，新增表时需评估是否加入排除列表
4. **`EnsureTenantID` 性能**：该方法在 feed-server 的每次请求中被调用，但通过本地 gcache + Redis 两级缓存，实际 DB 查询极少
5. **唯一索引调整**：部分表的唯一索引需要加入 `tenant_id` 字段（如 `template_spaces` 的 `idx_tenantID_bizID_name`），迁移时可能因重复数据导致冲突，需手动处理
