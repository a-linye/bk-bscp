# Context - 需求实现上下文

## 项目信息

- **项目名称**: bk-bscp
- **语言**: Go 1.21+
- **代码规范**: .golangci.yml
- **分支**: feature/adapt-template-variables-to-project1

## 关键文件路径

### Proto 定义
- `pkg/protocol/feed-server/feed_server.proto` - Feed 服务协议定义
  - AppMeta 消息（需扩展）
  - 所有 RPC 方法定义

### 核心类型定义
- `pkg/sf-share/types.go` - Sidecar/SDK 共享类型
  - SideAppMeta 结构体（需扩展）
  - BasicData 结构体
  - InstanceSpec 结构体

### Feed Server 拦截器层
- `cmd/feed-server/service/interceptor.go`
  - FeedEnsureTenantInterceptor - 租户解析
  - FeedUnaryAuthInterceptor - 鉴权中间件
  - extractBizIDAndApp - 提取 biz/app 信息

### Cache Service 缓存 Key
- `cmd/cache-service/service/cache/keys/cache.go`
  - element 结构体（需扩展）
  - 所有缓存 key 生成方法

### DAO 层
- `internal/dal/dao/project.go` - GetDefaultProject 方法
- `internal/dal/dao/environment.go` - GetDefaultEnvironment 方法（假设存在）

## 技术约束

1. **Proto 兼容性**: 只能新增 optional 字段，不能修改已有字段
2. **向后兼容**: 老客户端不传 project/env → 自动降级为默认值
3. **缓存策略**: 动态转换 key 格式，不需要双写
4. **校验规则**:
   - 老客户端: 只能访问 default project/default environment
   - 新客户端: 可访问任意项目/环境

## 已确认的决策

| 决策项 | 结论 |
|--------|------|
| 新字段命名 | project_id (uint32) + environment_id (uint32) |
| 默认值获取 | GetDefaultProject() + GetDefaultEnvironment() |
| 校验失败处理 | 返回明确错误 |
| 缓存迁移策略 | 动态转换 |
| 新客户端传参 | 都可选，不传则降级 |
| 同名 app | 允许跨项目/环境存在 |
