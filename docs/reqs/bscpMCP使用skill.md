# bscp skill 开发

## 基本信息

| 字段 | 值 |
|------|-----|
| 需求 ID | 1020451610135698141 |
| 需求短 ID | 135698141 |
| 需求名称 | bscp skill 开发 |
| 优先级 | 未指定 |
| 父需求 | 无 |
| 创建时间 | 2026-07-01 15:14:10 |
| 原始需求文档 | docs/reqs/bscpMCP使用skill.md |
| 预估工时 | 16 人时（2 人天） |
| 价值规模 | 50（Reach=20, Impact=5, Confidence=100%, Effort=2 人天） |

> **RICE 评分明细**：RICE = (Reach 20 × Impact 5 × Confidence 100%) / Effort 2 = 50（🟡 中，正常排期）。
> - Reach=20：特定角色/模块——使用 AI IDE 操作 bscp 配置的研发/运维
> - Impact=5：重要功能改进（提升 AI 辅助配置操作的可用性与可靠性）
> - Confidence=100%：需求澄清充分、领域知识经仓库代码核实、方案明确
> - Effort=2 人天：预估 16 人时 ÷ 8

## 需求背景

### 业务背景

bscp（蓝鲸基础配置平台）已在蓝鲸 API 网关上创建了对应的 MCP Server（`bk-bscp-prod-server-mcp`），
供 AI Agent/大模型通过 MCP 协议调用 bscp 的配置管理能力，当前暴露 14 个 KV 配置类工具。

**需求演进说明**：本需求最初的动机是"网关生成的 MCP 工具 schema 未带出 request body 字段描述，
模型难以填参"。在澄清过程中已从根因层面解决——通过重新同步 swagger 到蓝鲸 API 网关，
`Config_CreateKv`/`UpdateKv`/`BatchUpsertKvs`/`CreateRelease`/`Publish` 等工具的 body 字段描述
均已正确带出。因此本 skill **不再承担"补全字段描述"的职责**。

但字段描述完整并不等于模型会正确使用 bscp：MCP 工具 schema 只能描述"单个工具的单个字段"，
**无法表达跨工具的调用编排、字段间的业务约束、错误语义与领域模型**。这些正是模型使用 bscp
时反复踩坑的地方。因此本 skill 重新定位为 **"bscp KV 配置操作指引"**，为模型补充 MCP schema
无法表达的领域知识与操作规范。

> 定位：知识 + 调用指引型 skill，不改动网关 MCP，不重复 MCP 已有的字段描述。

### 用户故事

作为 使用 Cursor / Claude 等 AI IDE 并挂载了 bscp MCP 的研发人员
我想要 有一个 bscp skill 为模型补充调用编排、业务约束、错误处置与领域模型等 MCP schema 表达不了的知识
以便于 模型能按正确顺序完成"改 KV → 生成版本 → 发布 → 验证"闭环、遵守字段间的业务约束、遇错能自我纠正

### 需求来源

- **需求渠道**：技术优化 / 工具链体验改进
- **关联需求**：无
- **参考资料**：
  - MCP 工具现状：`bk-bscp-prod-server-mcp` 暴露的 14 个工具（body 字段描述已由网关补齐）
  - 领域知识来源：仓库代码（`cmd/data-service/service/kv.go`、`pkg/dal/table/kv.go`、`pkg/criteria/validator/name.go`、`pkg/cc/types.go` 等）
  - 已有 skill 组织规范参考：仓库 `.claude/skills/` 下现有 SKILL.md

## MCP 工具现状（14 个工具，字段描述已补齐）

> skill 直接引用 MCP 已有的字段描述，不再重复。下表说明各工具在 skill 中的定位。

| 工具 | 用途 | skill 中的角色 |
|------|------|--------------|
| Config_ListAppsBySpaceRest | 按 space 列 app | 参数获取（定位 app_id）；⚠️ 见"未解决问题 Q-001" |
| Config_GetAppByName | 按服务名取 app | 参数获取（已知服务名） |
| Config_CreateKv / UpdateKv / DeleteKv | 单条 KV 增改删 | 编排 + 业务约束 |
| Config_BatchUpsertKvs / BatchDeleteKv | 批量 KV 增改删 | 编排 + 业务约束（replaceAll / 按 ids 删） |
| Config_ListKvs | 列 KV（草稿区） | 编排（删除前拿 id） |
| Config_CreateRelease | 生成版本 | 编排（草稿转版本） |
| Config_GenerateReleaseAndPublish | 生成版本并发布（支持灰度） | 编排（灰度发布首选） |
| Config_Publish | 发布指定版本 | 编排（需已有 releaseId） |
| Config_ListReleases | 版本列表 | 编排 + 验证 |
| Config_GetReleasedKv / ListReleasedKvs | 查已发布 KV | 验证 |

## 功能需求

### 覆盖范围

本期 skill 面向 `bk-bscp-prod-server-mcp` 当前暴露的 14 个工具，聚焦 MCP schema 表达不了的知识层。

### 核心功能点

| 功能编号 | 功能描述 | 优先级 | 备注 |
|---------|---------|--------|------|
| F-001 | 领域模型速览：biz→app→kv→release 层级，KV 仅适用 config_type=kv 的 app | P0 | 必须 |
| F-002 | 端到端调用编排指引：定位 app → 改 KV（草稿）→ 生成版本 → 发布 → 验证 | P0 | 必须 |
| F-003 | 参数获取指引：biz_id 来源、app_id 通过 ListAppsBySpaceRest/GetAppByName 获取 | P0 | 必须 |
| F-004 | 字段级业务规则与约束（MCP schema 表达不了的关联约束） | P0 | 必须 |
| F-005 | 常见报错 → 原因 → 处置 对照表，支持模型自我纠错 | P0 | 必须 |
| F-006 | 端到端场景化示例（多工具编排序列） | P1 | 应该有 |

### 详细功能描述

#### [F-001] 领域模型速览

- biz（业务）→ app（服务，含 config_type）→ kv（草稿态配置项）→ release（不可变版本快照）
- KV 操作仅适用于 `config_type=kv` 的 app（对 file 型 app 操作会报 "not a KV type service"）
- KV 的增删改均为**草稿态**，客户端不可见，需生成版本并发布后才生效

#### [F-002] 端到端调用编排指引

- **闭环链路**：
  1. 定位服务：`ListAppsBySpaceRest` 或 `GetAppByName` → 拿 `appId`
  2. 改配置（草稿态）：`CreateKv`/`UpdateKv`/`DeleteKv`/`BatchUpsertKvs`/`BatchDeleteKv`
  3. 生成版本：`CreateRelease`（产出 releaseId）或一步式 `GenerateReleaseAndPublish`
  4. 发布：`Config_Publish`（需已有 releaseId）；灰度发布优先用 `GenerateReleaseAndPublish`
  5. 验证：`ListReleases`/`GetReleasedKv`/`ListReleasedKvs`
- **关键顺序约束**：
  - KV 增删改是草稿，必须"生成版本 + 发布"后才对客户端生效
  - `DeleteKv`/`BatchDeleteKv` 按 **id/ids** 删除（非 key）→ 需先 `ListKvs` 拿 id
  - `Config_Publish` 只接受**已有的 releaseId**，不会从草稿自动生成版本
  - `Publish` vs `GenerateReleaseAndPublish` 的取舍：前者发布已生成的版本；后者一步生成并发布、支持灰度

#### [F-003] 参数获取指引

- `biz_id`：来自蓝鲸平台上下文（CMDB 业务 / 空间），或请求头 `X-Bkapi-Biz-Id`；MCP 不提供列 biz 的工具
- `app_id`：通过 `ListAppsBySpaceRest`（按 biz 列 app）或 `GetAppByName`（已知服务名）获取；该工具当前未暴露 config_type 过滤，需从返回结果的 `config_type` 字段辨别 kv 型 app

#### [F-004] 字段级业务规则与约束

> 以下为 MCP schema 无法表达的关联约束（schema 仅描述单字段），以仓库代码为准。

- `key`：长度 1–128；仅中英文/数字/`_`/`-`，首尾须中英文数字；禁止 `_bk` 前缀；**不含 `.` 和 `/`**
- `value`：非空；**上限 1MB**；按 kvType 格式校验（json 须合法 JSON、yaml/xml 同理、number 须数字、string 不含换行）
- `kvType`：单条 KV **不可填 `any`**（any 仅用于 app.data_type）；必须与 app 的 data_type 匹配；**UpdateKv 不可改 kvType**
- `secret`：`secretType` 为枚举必填（password/certificate/secret_key/token/custom）；certificate 类型 value 须 X.509 PEM；`secretHidden` 控制明文可见性
- 数量上限：单 app 未删除 KV 默认上限 **2000**

#### [F-005] 常见报错 → 原因 → 处置 对照表

| 报错关键字 | 原因 | 处置 |
|-----------|------|------|
| already exists | key 重复 | 改用 UpdateKv 或换 key |
| kv type does not match... | kvType 与 app.data_type 不一致 | 按 app.data_type 修正 kvType |
| the type of config item ... is incorrect | 批量导入类型与已有不一致 | 保持已有类型 |
| not a KV type service | 对非 kv 型 app 操作 | 确认 config_type=kv |
| there is a release in publishing currently | 有版本正在上线 | 等待上线完成 |
| release ... is deprecated | 版本已废弃 | 换未废弃版本或重新生成 |
| exceeded the limit | 超 2000 上限 | 清理无用 KV |
| duplicate keys | 批量重复 key | 去重 |

#### [F-006] 端到端场景化示例

提供多工具编排的调用序列样例，如：
- 新增一个 json 配置并全量发布
- 批量导入 KV（含 replaceAll 语义）
- 更新一个 secret 配置
- 灰度发布到指定分组

## 非功能需求

### 可维护性

- 领域约束与操作规范采用**静态维护**方式：直接写入 skill 文档，随 bscp 领域规则变化由人工更新。
- 本期不引入自动生成/同步脚本。

### 兼容性

- 不改动网关侧 MCP Server 及其工具 schema。
- 不重复 MCP 已有的字段描述（引用即可）。
- skill 遵循仓库现有 `.claude/skills/` 下的 SKILL.md 规范，随仓库版本管理。

## 验收标准

### 功能验收

- [ ] **AC-001**：Given 用户要求"新增一个 KV 并让客户端生效"，When 模型按 skill 指引操作，Then 模型能按"定位 app → CreateKv → 生成版本 → 发布 → 验证"的正确顺序完成，且明确"草稿需发布才生效"
- [ ] **AC-002**：Given 用户要求删除某个 KV（只知道 key），When 模型按 skill 指引操作，Then 模型先 `ListKvs` 拿到 id 再 `DeleteKv`
- [ ] **AC-003**：Given 需要写入 `kvType=json`/`secret` 的 KV，When 模型按 skill 指引填参，Then 能满足 value 须合法 JSON、secret 须填 secretType 等约束
- [ ] **AC-004**：Given 需要灰度发布，When 模型按 skill 指引操作，Then 使用 `GenerateReleaseAndPublish`（groups/labels/grayPublishMode）完成灰度
- [ ] **AC-005**：Given MCP 返回常见错误（如 key already exists / 类型不匹配 / release 正在上线），When 模型查阅 skill 报错对照表，Then 能给出对应处置并自我纠正
- [ ] **AC-006**：Given 需要筛选 kv 型服务，When 模型按 skill 指引操作，Then 能通过 `ListAppsBySpaceRest` 列出 app 并从返回结果的 `config_type` 字段辨别、定位 kv 型 app

### 可维护性验收

- [ ] **AC-M01**：skill 存放于本仓库 `.claude/skills/` 下，符合现有 SKILL.md 规范；不重复 MCP 已有字段描述

## 边界范围

### 本期包含

- 领域模型速览、端到端调用编排、参数获取、字段级业务规则/约束、报错对照表、场景化示例。
- 以仓库代码为领域约束来源；引用 MCP 已有字段描述。
- skill 以静态维护方式落地在本仓库 `.claude/skills/` 下。

### 本期不包含

- 补全/重复 MCP 工具字段描述（已由网关 swagger 同步解决）。
- 修复 swagger / 网关同步（属运维/文档动作，非 skill 职责；见"未解决问题"）。
- 覆盖 bscp 全量 API（KV 配置闭环以外的接口，如模板、分组管理、客户端等）。
- 从 swagger 自动生成/同步内容的脚本或工具链。
- 改动网关侧 MCP Server 或其工具 schema。

## 约束条件

- **技术限制**：不改动网关 MCP；领域约束以仓库代码为准。
- **格式限制**：遵循仓库现有 `.claude/skills/` SKILL.md 规范。

## 未解决问题

| 问题 ID | 问题描述 | 状态 |
|--------|---------|------|
| Q-001 | `Config_ListAppsBySpaceRest` 网关 swagger 为过时分叉版本（GET+query，含 name/operator，缺 configType/topIds/search）；当前 proto/`api.swagger.json` 已定义为 POST+body 含 `configType`。**需求方决定网关暂不补 config_type 说明**，skill 通过读取 app 列表结果中的 `config_type` 字段辨别 kv 型 app 绕行处理 | 已定：网关暂不改，skill 绕行 |
| Q-002 | skill 具体目录命名与触发描述（description 关键词）留待实现阶段确定 | 已定：目录 `.claude/skills/bscp-kv-config/`，description 触发词覆盖"操作 bscp 配置/改 KV/发布版本/灰度发布"及 `Config_*` 工具名 |
| Q-003 | 灰度发布工具归属：需求文档假设灰度走 `Config_GenerateReleaseAndPublish`（groups/labels/grayPublishMode），但当前 MCP schema 中该工具未暴露 body 参数，灰度字段实际在 `Config_Publish` 上 | 已定：skill 据实指引灰度走 `Config_CreateRelease` + `Config_Publish`（all=false + grayPublishMode + groups/labels），`GenerateReleaseAndPublish` 定位为一步全量发布 |

---

## 原需求描述

> （TAPD 原始需求描述为空："(无描述内容)"）
>
> 补充说明（来自需求发起沟通）：当前 bscp 的 mcp 在蓝鲸网关上创建之后，由于 bscp 的 api 文档没有很好的维护，
> 导致工具 schema 没有把 request body 的字段描述带出来，这使得模型很难正常使用 mcp。因此需要开发一个 bscp 的 skill。

## 澄清记录

### 第 1 轮澄清（初版，基于 7 个 KV 工具）

**结论**：知识+调用指引型 skill；覆盖当时 7 个 KV 工具；字段来源 `bkapigw.swagger.json`；放本仓库 `.claude/skills/`；静态维护。

### 第 2 轮澄清（能力探讨）

**Agent 建议**：结合仓库代码补齐调用编排、参数获取、字段级业务规则、报错对照、示例、领域模型等 schema 表达不了的能力；并指出当时 MCP 缺 CreateRelease 导致闭环断裂。

### 第 3 轮澄清（MCP 扩容至 14 工具）

**背景变化**：MCP 从 7 扩到 14，补齐闭环；`Config_GenerateReleaseAndPublish2` 改名为 `Config_GenerateReleaseAndPublish`。
**结论**：覆盖全 14 工具；纳入全部能力；对遗留 schema 缺陷 skill 补全并绕行。

### 第 4 轮澄清（根因修复，需求重定位）

**背景变化**：需求方重新同步 swagger 到蓝鲸 API 网关，`CreateKv/UpdateKv/BatchUpsertKvs/CreateRelease/Publish` 的 body 字段描述已全部正确带出（含 Publish 灰度 body）。经查 `ListAppsBySpaceRest` 缺 config_type 的根因为网关使用了过时分叉的 swagger 定义，需求方将另行同步网关修复。

**用户决策**：
1. skill 继续做，但**重定位为"bscp KV 配置操作指引"**：去掉 body 字段补全，聚焦 MCP schema 表达不了的领域约束、调用编排、报错处置、示例、领域模型
2. `ListAppsBySpaceRest` 的 config_type 缺失：需求方决定网关暂不修改，由 skill 从返回结果的 `config_type` 字段辨别 kv 型 app 绕行处理（记入 Q-001）
