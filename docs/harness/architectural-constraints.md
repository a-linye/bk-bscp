# 架构约束（Architectural Constraints）

> 目标：在不放大历史耦合的前提下，保持服务边界、协议边界和数据访问边界清晰。

## 1. 系统视图

### 1.1 主要运行单元

| 运行单元 | 目录 | 职责 |
|---------|------|------|
| 管理端 | `ui/`、`cmd/ui/` | Vue 3 页面、静态资源与 Web 承载 |
| API Server | `cmd/api-server/` | HTTP API、路由、中间件和对内服务编排 |
| Config Server | `cmd/config-server/` | 配置管理领域服务与 gRPC 接口 |
| Data Service | `cmd/data-service/` | 持久化、事务、迁移、定时任务及数据访问编排 |
| Auth Server | `cmd/auth-server/` | 权限校验和认证服务 |
| Feed/Cache | `cmd/feed-server/`、`cmd/cache-service/`、`cmd/feed-proxy/` | 配置消费、匹配、缓存和推送链路 |
| Vault Server | `cmd/vault-server/` | 密钥与敏感配置相关能力 |

### 1.2 典型数据流

```text
UI / SDK / API Consumer
  → API Server / gRPC-Gateway
  → Config Server / Auth Server
  → Data Service
  → internal/dal
  → MySQL / Redis / BKRepo / object storage

发布与消费：
Data Service / Config Server
  → event / release
  → Feed Server / Cache Service / Feed Proxy
  → Sidecar / SDK / workload
```

具体调用关系以 `.proto`、服务装配代码和开发地图为准，不根据目录名臆测。

## 2. 分层与目录映射

依赖方向从入口和编排层流向协议、领域与基础设施边界；底层包不得反向依赖具体服务入口。

| 层 | 目录 | 职责 | 允许依赖 |
|----|------|------|---------|
| 展示与传输 | `ui/`、`cmd/*/service` 路由/handler | 页面、HTTP/gRPC 输入输出、鉴权中间件 | 应用编排、协议类型 |
| 应用编排 | `cmd/*/app`、`cmd/*/service`、`cmd/*/bll` | 用例编排、事务边界、服务生命周期 | 领域处理、接口、基础设施适配 |
| 领域与任务 | `internal/processor/`、`internal/task/`、`internal/expression/` | 业务规则、同步处理、任务构建与执行 | 稳定类型和抽象接口 |
| 基础设施适配 | `internal/dal/`、`internal/components/`、`internal/thirdparty/`、`internal/iam/` | 数据库、缓存和外部系统适配 | `pkg/` 稳定契约、第三方库 |
| 共享契约 | `pkg/protocol/`、`pkg/types/`、`pkg/criteria/`、`pkg/dal/table/` | 协议、公共类型、校验和表模型 | 标准库及必要基础依赖 |

该映射描述新增代码的目标边界，不宣称历史代码已完全满足。发现历史例外时，不在无关任务中重构，但禁止继续扩散。

## 3. 强制依赖规则

| 编号 | 规则 | 修复指引 |
|------|------|---------|
| ARCH-001 | `pkg/` 不得依赖 `cmd/` 或具体服务实现 | 将共享契约下沉到 `pkg/`，实现留在服务或 `internal/` |
| ARCH-002 | `internal/dal/` 不得调用 API handler 或服务入口 | 在应用层编排，通过 DAO/Repository 接口访问数据 |
| ARCH-003 | 服务间通信必须使用 `pkg/protocol/` 契约或明确客户端 | 新增/修改 `.proto`，生成代码后通过客户端调用 |
| ARCH-004 | UI 不得依赖后端内部实现细节 | 只依赖公开 API 契约和稳定响应模型 |
| ARCH-005 | 外部系统 SDK 细节不得散落在领域逻辑 | 封装在 `internal/components/`、`internal/thirdparty/` 或专用适配层 |
| ARCH-006 | 禁止新增循环依赖和跨服务复制同一业务规则 | 提取稳定公共规则或明确唯一服务所有权 |
| ARCH-007 | 生成文件不得手工维护 | 修改源 `.proto`/生成器并运行仓库生成命令 |

当前 `.golangci.yml` 负责通用静态检查，但未声明上述全部目录级规则。<!-- TODO: 待补充自动化架构测试或自定义 Linter，将 ARCH-001～ARCH-006 接入 CI。 -->

## 4. 数据边界

| 边界 | 原始输入 | 解析目标 | 处理位置 |
|------|---------|---------|---------|
| HTTP/API | Path、Query、Header、JSON/Form | 协议请求与强类型参数 | API Server handler/middleware |
| gRPC | Protobuf 消息 | 领域用例参数 | 各服务传输边界 |
| 配置文件/环境变量 | YAML、JSON、环境变量 | 服务配置结构 | 服务启动与配置加载阶段 |
| 数据库 | 行记录 | `pkg/dal/table` / DAO 模型 | `internal/dal/` |
| 外部系统响应 | JSON/SDK 类型 | 项目内部类型 | `internal/components/` 或 `internal/thirdparty/` |
| 文件上传/配置内容 | 字节流与元数据 | 校验后的内容、版本和存储引用 | API 边界与 Data Service |
| 事件/任务 | 消息、任务参数 | 已验证任务步骤 | `internal/task/`、Feed/Cache 链路 |

边界处完成必填项、枚举、长度、权限、租户和资源归属校验；内部层不以重复校验掩盖不可信输入未解析的问题。

## 5. 多租户、安全与一致性

- 所有资源访问必须保留 `biz_id`/space/app 等归属边界；涉及租户的路径必须正确传播 `tenant_id`。
- 鉴权在可信业务操作之前完成，禁止仅由 UI 隐藏按钮代替后端鉴权。
- 日志、错误和审计记录不得输出密钥、凭证、完整配置内容或个人敏感信息。
- 数据库迁移位于 `cmd/data-service/db-migration/`，必须可审查、可验证，并评估回滚或前向修复策略。
- 发布、事件和任务链路需考虑幂等、重试、乱序和重复消费，不以“通常只调用一次”为前提。

## 6. 架构决策记录

重大架构选择应新增到 `docs/adr/NNNN-标题.md`，至少记录背景、候选方案、决策、后果和状态。仓库当前尚未建立 ADR 目录；首次需要架构决策时创建，不为已有历史决策补造理由。

## 检查清单

- [ ] 新增依赖符合层次和服务边界
- [ ] 跨服务调用使用明确协议，不直接引用对方实现
- [ ] 外部系统和持久化细节封装在适配层
- [ ] 不可信输入在边界完成解析、鉴权和归属校验
- [ ] 生成代码由权威源重新生成
- [ ] 迁移、事件与任务处理评估了幂等和失败路径
- [ ] 重大架构变化已有 ADR 或设计文档
