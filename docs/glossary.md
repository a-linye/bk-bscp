# 词汇表（Glossary）

> bk-bscp 中 Agent 与开发者共用的核心术语。协议字段和既有英文标识保持源码命名。

## Harness Engineering 核心概念

| 术语 | 英文 | 定义 |
|------|------|------|
| 驾驭工程 | Harness Engineering | 通过上下文、约束、工具、执行验证和熵管理构建可靠的 Agent 运行环境。 |
| 上下文工程 | Context Engineering | 管理知识来源、加载顺序和动态数据，使 Agent 获取准确且适量的信息。 |
| 架构约束 | Architectural Constraints | 规定模块边界、依赖方向、数据入口和不可违反的工程规则。 |
| 熵管理 | Entropy Management | 持续检测并收敛代码、生成物、文档和依赖漂移。 |
| 工具能力 | Tooling | Agent 可调用的 Skill、MCP、CLI 及其权限和稳定性约束。 |
| 执行与验证 | Execution & Verification | 以观察、行动、验证和状态更新组成的任务闭环。 |

## BSCP 业务术语

| 术语 | 英文/标识 | 定义 |
|------|-----------|------|
| 服务配置中心 | BSCP | BlueKing Service Configuration Provider，为业务提供配置管理、版本和消费能力。 |
| 业务空间 | Space / `biz_id` | 配置资源的顶层业务隔离边界。 |
| 服务 | App | BSCP 中承载配置、分组、版本和凭证的核心业务单元。 |
| 配置项 | Config Item | 文件型配置的元数据及内容引用。 |
| KV 配置 | KV | 以键值对形式管理和发布的配置。 |
| 配置模板 | Config Template | 可复用并绑定到服务的配置结构。 |
| 模板修订 | Template Revision | 配置模板在某次编辑后的不可混淆版本。 |
| 分组 | Group | 按规则划分实例、主机或客户端的发布目标集合。 |
| 灰度发布 | Gray Release | 将新版本按分组或规则逐步发布到部分目标。 |
| 发布版本 | Release | 一组配置在某个时间点形成的可发布快照。 |
| Feed | Feed | 向客户端匹配、缓存并分发已发布配置的消费链路。 |
| Sidecar | Sidecar | 与业务工作负载协同运行并拉取/落地配置的组件形态。 |
| 凭证 | Credential | 客户端访问 BSCP 服务或配置的身份凭据。 |
| 模板变量 | Template Variable | 模板渲染时由服务、环境或用户提供的变量。 |
| 操作范围表达式 | Operate Range Expression | 描述进程、主机或实例操作集合的结构化表达式。 |

## 架构与协议

| 术语 | 英文/缩写 | 定义 |
|------|-----------|------|
| API Server | API Server | 对外 HTTP API、路由和中间件入口。 |
| Config Server | Config Server | 承载配置管理领域接口和应用编排的服务。 |
| Data Service | Data Service | 承载持久化、迁移、定时任务和数据访问的服务。 |
| Auth Server | Auth Server | 负责认证和权限校验的服务。 |
| gRPC-Gateway | gRPC-Gateway | 将 Protobuf/gRPC 契约映射为 HTTP API 的网关机制。 |
| Protobuf | Protocol Buffers | `pkg/protocol/` 中接口与消息的权威定义格式。 |
| DAL | Data Access Layer | `internal/dal/` 中封装数据库、缓存和仓储访问的层。 |
| DAO | Data Access Object | 面向具体数据实体的访问对象。 |
| BLL | Business Logic Layer | 部分服务中组织业务规则与用例的逻辑层。 |
| 多租户 | Multi-tenancy | 通过租户和业务归属字段隔离资源与访问。 |
| 幂等 | Idempotency | 同一操作重复执行不会产生额外副作用。 |

## Agent 与工程实践

| 术语 | 英文/标识 | 定义 |
|------|-----------|------|
| Skill | Skill | 包含触发条件、领域约束和执行流程的 Agent 能力文档。 |
| MCP | Model Context Protocol | Agent 发现并调用外部服务工具的协议。 |
| 开发地图 | Dev Map | `graphify` 生成的项目知识图谱及查询入口。 |
| 文档园艺 | Documentation Gardening | 检查并修复文档与项目实际状态不一致的维护过程。 |
| 权威源 | Single Source of Truth | 某类事实的唯一版本受控来源，如 `.proto` 对生成协议。 |
| ADR | Architecture Decision Record | 记录重大架构决策背景、方案、后果和状态的文档。 |
| TDD | Test-Driven Development | 先用失败测试描述行为，再实现并重构的开发方式。 |
| Spec Kit | Spec Kit | 生成和校验 specification、plan、tasks 等开发产物的工具集。 |
| 工作区 | Workspace | 当前 Git 仓库及其未提交状态；也可能在 TAPD 中表示 `workspace_id`，需结合上下文区分。 |

## 状态与信号

| 信号 | 格式 | 定义 |
|------|------|------|
| 工具结果 | `success/data/error` | 推荐的结构化工具输出语义。 |
| 任务状态 | `pending/in_progress/completed/blocked` | 执行清单中任务的生命周期状态。 |
| 检查严重级别 | `P0/P1` | P0 为权威源明确、可机械修复的偏差；P1 需要业务或架构决策。 |
| 验证结果 | `pass/fail/skipped` | 验证通过、失败或有明确理由跳过。 |
| 环境状态 | `ready/missing/not-probed` | 工具已就绪、缺失或当前场景未探测。 |

---

*持续补充中：引入新业务概念、协议字段或工作流信号时，在对应分类中同步更新。*
