# 模板变量适配到项目维度（Feed 协议扩展与缓存 Key 迁移）

## 基本信息

| 字段 | 值 |
|------|-----|
| 需求 ID | 135156310 |
| 需求名称 | 模板变量适配到项目维度 |
| 优先级 | High |
| 父需求 | 无 |
| 创建时间 | 2026-07-13 |
| 原始需求文档 | docs/reqs/模板变量适配项目维度.md |

## 需求背景

### 业务背景

当前 BSCP 系统中，配置管理以**业务（Biz）+ 应用（App）**为两级维度进行组织。随着业务发展，需要在业务和应用之间引入**项目（Project）**和**环境（Environment）**两个中间维度，实现更细粒度的配置隔离和管理：

- **多项目支持**：同一业务下可创建多个项目，不同项目间配置完全隔离
- **多环境支持**：同一项目下可存在多个环境（如 dev/staging/prod），每个环境可独立配置
- **向后兼容**：已部署的老客户端无需升级或调整配置，继续按默认项目/默认环境拉取原有配置

### 用户故事

**作为** BSCP 平台运维人员
**我想要** 在 Feed 协议中支持项目和环境维度的配置拉取
**以便于** 实现多项目、多环境下的配置管理，同时不影响已部署的老客户端正常使用

### 需求来源

- **需求渠道**：技术优化
- **关联需求**：无
- **参考资料**：本仓库 `pkg/sf-share/types.go`、`pkg/protocol/feed-server/feed_server.proto`

## 功能需求

### 核心功能点

| 功能编号 | 功能描述 | 优先级 | 涉及角色 | 备注 |
|---------|---------|--------|---------|------|
| F-001 | Proto 协议扩展：新增 project_id / environment_id 可选字段 | P0 | feed-server | 必须有 |
| F-002 | 老客户端兼容：不传 project/env 时降级为默认项目/环境 | P0 | feed-server | 必须有 |
| F-003 | 缓存 Key 扩展：从 `biz+app` 扩展为 `biz+project+env+app` | P0 | cache-service | 必须有 |
| F-004 | 校验逻辑：老客户端传入的 app 必须属于默认项目/环境 | P1 | feed-server | 应该有 |
| F-005 | 新客户端支持：显式传递 project_id 和 environment_id | P1 | feed-server/sidecar | 应该有 |

### 详细功能描述

#### [F-001] Proto 协议扩展

- **输入**：修改 `feed_server.proto` 中的 `AppMeta` 消息定义
- **处理逻辑**：
  1. 在 `AppMeta` 消息中新增两个可选字段：
     ```protobuf
     message AppMeta {
       string app = 1;           // 已有字段，不变
       string uid = 2;           // 已有字段，不变
       map<string, string> labels = 3;  // 已有字段，不变
       uint32 project_id = 4;    // 新增：项目 ID（可选）
       uint32 environment_id = 5;// 新增：环境 ID（可选）
     }
     ```
  2. 同步在 `pkg/sf-share/types.go` 的 `SideAppMeta` 结构体中新增对应字段
- **输出**：更新后的 proto 文件和生成的 Go 代码
- **边界条件**：
  - 新增字段均为 `optional`，不传时值为 0
  - 不改变任何已有字段的含义和位置
- **异常处理**：proto 编译失败则终止构建

#### [F-002] 老客户端兼容降级

- **输入**：客户端请求（可能不含 project_id / environment_id）
- **处理逻辑**：
  1. feed-server 拦截器检测请求中的 `project_id` 和 `environment_id`
  2. 如果任一字段为空（0）：
     - 调用 `GetDefaultProject(bizID)` 获取默认项目 ID
     - 调用 `GetDefaultEnvironment(bizID)` 获取默认环境 ID
     - 将获取到的值注入到后续处理的 context 中
  3. 如果字段非空，直接使用客户端传递的值
- **输出**：context 中包含明确的 `project_id` 和 `environment_id`
- **边界条件**：
  - 默认项目/环境不存在时返回错误
  - 只传 project_id 不传 environment_id → 使用该项目的默认环境
  - 只传 environment_id 不传 project_id → 使用默认项目的指定环境
- **异常处理**：
  - 默认项目/环境不存在 → 返回错误码并提示"请先初始化默认项目/环境"

#### [F-003] 缓存 Key 扩展

- **输入**：biz_id、project_id、environment_id、app_id
- **处理逻辑**：
  1. 扩展现有的 `element` 结构体，增加 `project` 和 `environment` 字段：
     ```go
     type element struct {
       biz         uint32
       project     uint32  // 新增
       environment uint32  // 新增
       ns          namespace
       key         string
     }
     ```
  2. 更新所有涉及 app 的缓存 key 生成方法签名：
     - `ClientMetricKey(bizID, projectID, envID, appID)`
     - `ReleasedGroup(bizID, projectID, envID, appID)`
     - `AppMeta(bizID, projectID, envID, appID)`
     - `PublishString(bizID, projectID, envID, appID)`
     - `AppID(bizID, projectID, envID, appName)`
  3. **动态转换策略**：
     - 老客户端请求（project=0, env=0）：在调用缓存方法前，先解析出默认 project/env，再构造新格式 key
     - 新客户端请求：直接使用传递的 project/env 构造 key
- **输出**：新的缓存 key 格式：`{biz}bscp:{ns}:{project}:{env}:{key}`
- **边界条件**：
  - 老 key 格式 `{biz}bscp:{ns}:{key}` 在 TTL 过期后自然淘汰
  - 不需要双写策略，避免复杂度
- **异常处理**：无特殊异常，key 只是格式变更

#### [F-004] 校验逻辑

- **输入**：老客户端请求（不含 project_id / environment_id）
- **处理逻辑**：
  1. 当老客户端通过 app_id 查询应用时
  2. 解析出默认项目和默认环境
  3. 校验该 app_id 是否属于 `(default_project, default_environment)` 组合
  4. 如果不属于 → 返回错误："应用 xxx 不属于默认项目/环境，请使用新客户端或升级配置"
- **输出**：校验通过或错误信息
- **边界条件**：
  - 老客户端只传 app name → 只在 default project/default environment 内按 name 查找
  - 非 default 项目/环境的 app → 明确拒绝老客户端访问
- **异常处理**：返回明确错误信息，提示用户升级客户端或检查配置

#### [F-005] 新客户端显式传参

- **输入**：新客户端请求（包含 project_id / environment_id）
- **处理逻辑**：
  1. 新客户端可以在 `AppMeta` 或 `SideAppMeta` 中显式设置 `project_id` 和 `environment_id`
  2. 服务端校验：
     - 项目必须属于当前业务
     - 环境必须属于当前项目
  3. 允许不同项目/环境下存在同名 app
- **输出**：对应项目/环境下的配置数据
- **边界条件**：
  - project_id 和 environment_id 都可选（选项 C）
  - 不传 → 降级为默认项目/环境
  - 传了但无效 → 返回参数错误
- **异常处理**：
  - 项目不存在 → "项目不存在"
  - 环境不属于该项目 → "环境 xxx 不属于项目 xxx"

## 非功能需求

### 性能需求

- **响应时间**：默认项目/环境查询增加的延迟 < 5ms（需加本地缓存）
- **并发能力**：不支持对现有 QPS 产生明显影响（< 5% 降幅）

### 兼容性

- **接口兼容**：
  - ✅ Proto 只新增 optional 字段，不修改已有字段
  - ✅ 已部署老客户端无需升级，零配置变更继续使用
  - ✅ 新字段位置追加到 AppMeta/SideAppMeta 末尾
  - ❌ 不复用或改变已有字段含义

- **数据兼容**：
  - 老客户端统一落到 default project / default environment
  - 原 `biz + app` 的拉取结果保持不变

### 可用性与稳定性

- **降级方案**：默认项目/环境查询失败时不影响服务启动，仅影响相关请求
- **灰度策略**：可以先在新项目中验证，老项目不受影响

## 业务规则

### 业务逻辑规则

- **规则 R-001**：老客户端识别规则
  - 判断条件：请求中 `project_id == 0 && environment_id == 0`
  - 处理方式：自动填充默认项目和默认环境

- **规则 R-002**：默认项目/环境确定
  - 通过 `GetDefaultProject(bizID)` 和 `GetDefaultEnvironment(bizID)` API 获取
  - 每个 biz 有且只有一个默认项目（is_default=true）
  - 每个 biz 有且只有一个默认环境

- **规则 R-003**：兼容边界明确化
  - ✅ 老客户端只访问 default project / default environment
  - ❌ 非 default 项目/环境必须使用新客户端
  - ⚠️ 不同项目/环境下允许同名 app 存在

### 数据校验规则

- **必填字段**：biz_id（已有）、app 或 app_id（已有）
- **新增可选字段**：project_id、environment_id
- **取值范围**：project_id > 0 且属于当前 biz；environment_id > 0 且属于当前 project

## 外部依赖与集成

### 数据模型

#### 现有结构（需扩展）

```protobuf
// feed_server.proto - AppMeta（当前）
message AppMeta {
  string app = 1;
  string uid = 2;
  map<string, string> labels = 3;
}

// sf-share/types.go - SideAppMeta（当前）
type SideAppMeta struct {
  AppID     uint32            `json:"appID"`
  App       string            `json:"app"`
  Namespace string            `json:"namespace"`
  Uid       string            `json:"uid"`
  Labels    map[string]string `json:"labels"`
  Match     []string          `json:"match"`
  // ... 其他状态字段
}
```

#### 目标结构（扩展后）

```protobuf
// feed_server.proto - AppMeta（扩展后）
message AppMeta {
  string app = 1;
  string uid = 2;
  map<string, string> labels = 3;
  uint32 project_id = 4;      // 新增
  uint32 environment_id = 5;  // 新增
}

// sf-share/types.go - SideAppMeta（扩展后）
type SideAppMeta struct {
  AppID         uint32            `json:"appID"`
  App           string            `json:"app"`
  Namespace     string            `json:"namespace"`
  Uid           string            `json:"uid"`
  Labels        map[string]string `json:"labels"`
  Match         []string          `json:"match"`
  ProjectID     uint32            `json:"projectID"`     // 新增
  EnvironmentID uint32            `json:"environmentID"` // 新增
  // ... 其他状态字段
}
```

#### 缓存 Key 变更

```go
// 当前格式
{bizID}bscp:{namespace}:{key}

// 目标格式
{bizID}bscp:{namespace}:{projectID}:{envID}:{key}
```

### 接口契约

#### 涉及的 RPC 方法列表

| 方法名 | 参数位置 | 影响 |
|--------|---------|------|
| `PullAppFileMeta` | `AppMeta app_meta` | 需扩展 |
| `PullKvMeta` | `AppMeta app_meta` | 需扩展 |
| `GetKvValue` | `AppMeta app_meta` | 需扩展 |
| `GetSingleKvValue` | `AppMeta app_meta` | 需扩展 |
| `GetSingleFileContent` | `AppMeta app_meta` | 需扩展 |
| `Watch` | `SideWatchPayload` (JSON) | 需扩展 SideAppMeta |
| `Messaging` | `HeartbeatPayload` (JSON) | 需扩展 SideAppMeta |

#### 涉及的 DAO 方法

| 方法 | 用途 | 状态 |
|------|------|------|
| `GetDefaultProject(bizID)` | 获取默认项目 | ✅ 已存在 |
| `GetDefaultEnvironment(bizID)` | 获取默认环境 | ✅ 已存在 |

## 验收标准

### 功能验收

- [ ] **AC-001**：Given 已部署的老客户端 When 发起配置拉取请求（不含 project/env）Then 系统自动使用默认项目/环境返回正确配置
- [ ] **AC-002**：Given 新客户端 When 显式传递有效的 project_id 和 environment_id Then 返回对应项目/环境下的配置
- [ ] **AC-003**：Given 老客户端 When 传入一个不属于默认项目/环境的 app_id Then 返回明确错误提示升级客户端
- [ ] **AC-004**：Given 新客户端 When 只传 project_id 不传 environment_id Then 使用该项目的默认环境
- [ ] **AC-005**：Given 新旧客户端共存场景 When 同时发起请求 Then 互不影响，各自获得正确结果
- [ ] **AC-006**：Given 不同项目下存在同名 app When 分别拉取 Then 返回各自项目对应的配置
- [ ] **AC-007**：Given proto 文件 When 编译 Then 新增字段为 optional，旧客户端可正常反序列化

### 性能验收

- [ ] **AC-P01**：Given 正常流量 When 启用默认项目/环境解析 Then P99 响应时间增长 < 10ms

### 兼容性验收

- [ ] **AC-S01**：Given 未升级的老客户端 When 连接新版 feed-server Then 无需任何配置变更即可正常工作

## 边界范围

### 本期包含

- ✅ Feed Proto 协议扩展（AppMeta / SideAppMeta 新增字段）
- ✅ feed-server 拦截器层默认项目/环境解析
- ✅ cache-service 缓存 key 格式扩展
- ✅ 应用查询逻辑适配（按 project + environment 过滤）
- ✅ 老客户端兼容性保障

### 本期不包含

- ❌ UI 界面的项目/环境选择功能（如有另外的需求覆盖）
- ❌ 配置模板变量本身的语法变更
- ❌ data-service 的项目 CRUD API（已存在）
- ❌ sidecar 版本检测和强制升级机制
- ❌ 非 Feed 协议的其他接口改造（如 management API）

## 约束条件

- **技术限制**：
  - Proto3 语法，必须保持向后兼容
  - Go 1.21+，符合仓库 `.golangci.yml` 规范
  - 不能引入破坏性变更
- **兼容限制**：
  - 已部署的老客户端不能因本次变更而失败或行为改变
  - 缓存 key 变更需要平滑过渡，不能导致缓存击穿
- **时间限制**：无硬性上线时间要求

## 未解决问题

| 问题 ID | 问题描述 | 截止日期 | 状态 |
|--------|---------|---------|------|
| （暂无） | | | |

---

## 原需求描述

> 1. tapd单据135156310需求澄清。
> 2. 旧客户端协议不能破坏。feed proto 只能新增可选字段，不能改变现有字段含义。已经部署的客户端不需要升级或调整配置，继续按 default project/default environment 拉取原有配置。
> 3. 老 sidecar 当前以 SideWatchPayload.BizID + SideAppMeta 识别应用，SideAppMeta 中包含 AppID、App、Namespace、Uid 等字段。兼容规则为：
> 
> - 老客户端不传 project/env -> feed-server 解析到 default project/default environment
> - 老客户端传 app_id -> 校验 app_id 属于 default project/default environment
> - 老客户端只传 app name -> 只在 default project/default environment 内按 app name 查找
> - 非默认项目或非默认环境 -> 必须使用新客户端或新配置显式传 project/env
> 
> 4. 新客户端可以显式传：
> 
> - project_id/project_key
> - environment_id/environment_key
> 
> 5. 这些字段可以追加到 SideAppMeta / AppMeta，也可以在没有 AppMeta 的下载类请求中追加为可选字段；不能复用或改变已有字段含义。
> 
> 6. 缓存 key、事件 key、下载任务 key、客户端状态 key 需要扩展：
> 
> - 旧逻辑: biz_id + app_id
> - 新逻辑: biz_id + project_id + environment_id + app_id
> 
> 7. 老客户端统一落到默认项目和默认环境，所以原有拉取结果保持不变。
> 
> 8. 如果未来允许不同项目或不同环境下存在同名 app，老客户端无法区分项目和环境。兼容边界应明确为：
> 
> - 老客户端只访问 default project/default environment
> - 非 default 项目和环境必须使用新客户端或新配置

## 澄清记录

### 第 1 轮澄清

**提问时间**：2026-07-13 17:11

**Agent 提问**：

🔴 **阻塞性问题**（无）

🟡 **建议确认的问题**：

1. **[How-001]** 新字段的命名规范
   > 背景：需要确定新增字段的准确命名
   
2. **[How-002]** 默认项目和默认环境的确定逻辑
   > 背景：老客户端不传时如何获取默认值
   
3. **[How-003]** 校验失败的处理策略
   > 背景：老客户端传入非法 app_id 时的处理方式
   
4. **[How-004]** 缓存 key 扩展的迁移策略
   > 背景：新旧 key 格式的过渡方案
   
5. **[How-005]** 新客户端必传 vs 可选
   > 背景：新客户端是否强制要求传 project/env
   
6. **[Data-001]** 项目和环境的唯一性约束
   > 背景：是否允许同名 app 跨项目/环境存在

**用户回复**：

1. **How-001**：目前项目只有 `project_id` 没有 `project_key`，环境有 `environment_id` 和 `name`。结论：新增字段命名为 `project_id`（uint32）和 `environment_id`（uint32）

2. **How-002**：C 有提供 `GetDefaultEnvironment` 和 `GetDefaultProject`。结论：使用现有的 DAO 方法获取默认项目/环境

3. **How-003**：A（返回错误）。结论：校验失败时返回明确错误信息，提示客户端升级或检查配置

4. **How-004**：B（动态转换）。结论：直接切换新 key 格式，老客户端请求时动态转换为旧 key 格式，不需要双写

5. **How-005**：C（都可选）。结论：project_id 和 environment_id 都为可选字段，不传则降级为默认

6. **Data-001**：允许不同项目/环境下存在同名 app。结论：多项目/多环境的核心价值是隔离，同名 app 是合理场景
