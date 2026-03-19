# 多租户开发规范

## 基本要求
- 所有功能都要兼容多租户模式
- 如果参数中需要传递租户ID，将租户ID放在第一个参数中（如果有 kit 或 context 则放在它们后面作为第二个参数）
  - 示例: `func DoSomething(tenantID string, bizID uint32)` 
  - 示例: `func DoSomething(ctx context.Context, tenantID string, bizID uint32)`
  - 示例: `func DoSomething(kit *kit.Kit, tenantID string, bizID uint32)`
- `biz_id` 在系统中是全局唯一递增的，不同租户不会有相同的 `biz_id`

## GORM Hook 机制
- 项目已在 `internal/dal/dao/set_tenant_id.go` 中实现 GORM hook，自动处理多租户过滤
- 查询时自动添加 `WHERE tenant_id = ?` 条件
- 创建/更新时自动设置 `tenant_id` 字段
- **关键**: hook 从 `db.Statement.Context` 中通过 `kit.FromGrpcContext()` 读取 TenantID，而不是直接从 kit struct 读取
- 因此 `kit.Ctx` 必须通过 `InternalRpcCtx()` 或 `RpcCtx()` 将 TenantID 序列化到 gRPC metadata 中
- 当 `kt.TenantID` 为空时，hook 注入 `tenant_id IN ('default', '')`（兼容旧数据）
- 当 `kt.TenantID` 非空时，hook 注入 `tenant_id = 'xxx'`
- `excludedTables` 中的表（如 `clients`、`client_events`）不参与租户过滤，新增排除表需评估

## Kit 使用规范

### 创建 Kit
- **`kit.New()`**: TenantID 为空，Ctx 为 `context.Background()`。注意这**不是**跳过租户过滤，GORM hook 会注入 `tenant_id IN ('default', '')`，即只能查到 default 租户的数据
- **`kit.NewWithTenant(tenantID)`**: 创建带指定 TenantID 的 Kit，内部自动调用 `InternalRpcCtx()` 确保 Ctx 正确。**推荐在异步任务、回调等需要明确租户的场景使用**
- **`kit.New().WithSkipTenantFilter()`**: 跳过租户过滤，GORM hook 不会注入 tenant_id 条件。**仅用于系统级跨租户操作**（参见下方使用约束）
- **`kit.FromGrpcContext(ctx)`**: 从 gRPC context 提取 Kit，用于 RPC handler 中

### 修改 TenantID 后必须更新 Ctx
直接修改 `kt.TenantID` 后，**必须**同步更新 `kt.Ctx`，否则 GORM hook 读到的仍是旧值：
```go
kt.TenantID = "some-tenant"
kt.Ctx = kt.InternalRpcCtx()  // 必须！将 TenantID 序列化到 context 中
```

### WithSkipTenantFilter 使用约束
`kt.WithSkipTenantFilter()` 仅限以下系统级场景使用，**禁止**在业务查询中使用：
- 事件循环：`ListEventsMeta`、`RecordCursor`、`GetCurrentCursorReminder`
- 租户反查：`GetOneAppByBiz`（通过 biz_id 查租户，鸡蛋问题）
- 系统聚合：`GetAllBizs`、`Purge`（清理过期事件）
- 全量同步：`repo_syncer.syncAll`（bkrepo 仓库同步）
- 跨租户策略：事件消费中的 `GetStrategyByIDs`

## 各服务链路的租户传播

### api-server → config-server → data-service
- api-server 从 HTTP Header `X-Bk-Tenant-Id` 读取 TenantID 设置到 kit 中
- 通过 gRPC metadata 自动传播到下游服务，无需额外处理
- Dev 环境通过 `initKitWithDevEnv` 支持 Header 注入（`internal/iam/auth/middleware.go`）

### feed-server（sidecar 请求链路）
- sidecar 客户端通过 Bearer Token 认证，**不携带** `X-Bk-Tenant-Id`
- **必须**在处理请求前调用 `EnsureTenantID(kt, bizID)` 从 biz_id 反查租户
- 已实现的调用点：`Handshake`、`PullAppFileMeta`、`PullKvMeta`、`GetKvValue`、`DownloadFile`、`authorize`
- **新增 feed-server RPC handler 或 HTTP handler 时，必须在业务逻辑前调用 `EnsureTenantID`**
- `EnsureTenantID` 使用两级缓存（本地 gcache → Redis DB=1 → DB 查询），性能影响极小

### cache-service（事件消费链路）
- 事件循环入口使用 `WithSkipTenantFilter()`（事件是全局资源）
- 消费具体事件时，从 `event.Attachment.BizID` 解析出 TenantID，传入后续 DAO 操作
- **新增事件消费逻辑时，必须确保从事件中提取租户上下文**

### 异步任务（Task 执行器）
- Task 的 Payload struct 必须包含 `TenantID string` 字段
- Builder 创建任务时从 `kt.TenantID` 传入 Payload
- Executor 执行时使用 `kit.NewWithTenant(payload.TenantID)` 而不是 `kit.New()`
- **禁止**在 Task 执行器中使用 `kit.New().WithSkipTenantFilter()` 代替正确传递 TenantID
- 回调通知（`CallbackNotify`）同样需要携带 TenantID

## 定时任务多租户处理
- 定时任务使用 `kit.New()` 创建的 kit 默认没有 TenantID
- 多租户模式下需要按租户轮询执行：
  - 进程管理相关同步（sync_cmdb、cmdb_resource_watcher）：使用 `bkuser.ListEnabledTenants()` 获取全量租户
  - 业务主机同步（sync_biz_host）：使用 `App.GetDistinctTenantIDs()` 从 app 表获取租户
- 示例模式：
```go
if cc.DataService().FeatureFlags.EnableMultiTenantMode {
    tenants, _ := bkuser.ListEnabledTenants(kt.Ctx)
    for _, tenant := range tenants {
        kt.TenantID = tenant.ID
        kt.Ctx = kt.InternalRpcCtx()  // 必须同步更新 Ctx
        // 执行租户相关操作
    }
}
```

## 表结构要求
- 新增表必须包含 `tenant_id` 字段
- `tenant_id` 应作为联合主键或唯一索引的一部分
- 参考 `processes`、`process_instances` 等表的设计
- 如果新表不需要租户过滤（如 metrics 表），需显式加入 `set_tenant_id.go` 的 `excludedTables`

## 缓存与数据迁移
- Redis DB=1 中缓存了 biz_id → tenant_id 的映射，key 格式：`{biz_id}bscp:tenant-id:tenant-id`
- 数据迁移修改 `tenant_id` 后，**必须**清理对应的 Redis 缓存和服务内存缓存（需重启服务清理 gcache）
- feed-server 和 cache-service 使用 gcache（进程内缓存），修改租户映射后需重启这两个服务

## 外部 API 调用
- 调用蓝鲸组件 API 时需要在请求头中携带租户信息：`X-Bk-Tenant-Id: {tenantID}`
- 参考 `internal/components/bkuser/bkuser.go` 中的实现

## 常见陷阱
1. **直接改 kt.TenantID 忘记更新 Ctx**: GORM hook 读的是 Ctx 中的 metadata，不是 struct 字段
2. **新 RPC handler 忘记 EnsureTenantID**: feed-server 的所有入口都需要先解析租户
3. **Task Payload 忘记加 TenantID**: 异步执行时 kit 是新建的，必须从 Payload 恢复
4. **用 WithSkipTenantFilter 偷懒**: 这会跳过租户隔离，仅限系统级操作使用
5. **数据迁移忘记清缓存**: Redis 和 gcache 会持有旧的租户映射，导致查询结果不正确