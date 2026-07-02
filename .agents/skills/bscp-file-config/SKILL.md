---
name: bscp-file-config
slug: bscp-file-config
version: 1.0.0
description: |
  bscp（蓝鲸基础配置平台）文件型配置操作指引。为挂载了 bk-bscp-prod-file-manage 的模型补充
  MCP 工具 schema 表达不了的领域知识：文件型（config_type=file）配置的领域模型、端到端
  调用编排、参数获取、字段级业务约束、报错处置与场景化示例，帮助模型正确完成
  "改文件配置项元数据 → 生成版本 → 发布 → 取下载 URL 验证" 闭环。
  Use this skill whenever the user asks to 操作 bscp 文件配置, 改 bscp 文件型配置,
  新增/更新/删除文件配置项, 发布文件型服务版本, 文件配置灰度发布, 文件配置查询,
  看看某个文件型服务的配置, 取文件下载 URL 验证发布, 帮文件型服务发版,
  or invokes bk-bscp-prod-file-manage 的文件型配置项 / 版本 / 发布 / 下载 URL 工具。
metadata:
  requires:
    mcps: ["bk-bscp-prod-file-manage"]
---

# bscp 文件型配置操作指引

## 定位

本 skill 只补充 MCP schema 表达不了的知识（跨工具编排、字段间业务约束、错误语义、领域模型），
**不重复** MCP 工具已有的单字段描述——填参时字段含义以 MCP 工具 schema 为准。文中的业务
约束来自 bscp 服务端的实际校验规则，会随 bscp 版本演进；若与调用返回的报错不一致，以服务端返回为准。

适用范围：聚焦 `config_type=file` 服务的文件型配置闭环——**配置项元数据增删改 → 生成版本 →
发布 → 用下载 URL 验证**。**本期不含文件内容上传**（二进制上传仍走 UI / SDK，AI 只引用已上传内容的
sign）。不覆盖模板型（template）、KV 型、分组管理、客户端等文件闭环以外的接口。

## 前置条件

本 skill 依赖 `bk-bscp-prod-file-manage`（蓝鲸 API 网关提供的 bscp **文件型专用** MCP Server）暴露的工具。
文件型闭环所需的全部工具（服务定位、文件配置项增删改、生成版本、发布、下载 URL）都由这一个 MCP 提供，
与 KV 型的 `bk-bscp-prod-server-mcp` 相互独立。**开始操作前必须先确认这些工具已可用**：

1. 检查当前会话是否已挂载 `bk-bscp-prod-file-manage`，能看到 `Config_ListAppsBySpaceRest` /
   `Config_GetAppByName` / `Config_CreateRelease` / `Config_Publish` 等工具。
2. 文件型编排还需要「文件配置项」相关工具（列表 / 增删改）与「文件下载 URL」工具，同样由
   `bk-bscp-prod-file-manage` 提供。**这些工具须在蓝鲸 API 网关注册后才会出现在 MCP 工具集里**
   （下载 URL 接口即本需求 F-008 新注册项）。
3. **若某个工具不存在**：不要臆造工具名或伪造调用结果，直接告知用户"未检测到 `bk-bscp-prod-file-manage`
   或对应工具，可能尚未挂载 / 在网关注册"，并说明缺失的能力，然后停下等待用户确认接入，或退化为对可用工具的编排。
4. 工具可用后再按后续章节执行。

> 说明：文件下载 URL 接口是本期新增的管理面接口（只返回临时预签名 URL 与有效期，不透传文件字节），
> 网关注册后自动纳入 `bk-bscp-prod-file-manage` 工具集，用于**发布后验证**环节。

## 交互引导（面向不熟悉闭环的用户）

用户往往只抛一个模糊意图（如"看看某文件服务的配置 / 改一下发布"）而不知道要给哪些参数、走什么闭环。
**不要一次性罗列一堆参数把用户劝退，也不要臆造参数**；按下面的方式**分步反问，一次只问当前缺的一个关键信息**。

### 通用引导步骤（任何意图先做）

1. **确认业务 ID（bizId）**：用户没给就先问"请提供业务 ID（bizId）"。
2. **确认服务名 → 解析 appId**：拿到服务名后调 `Config_GetAppByName`（或先 `Config_ListAppsBySpaceRest`
   让用户从列表里挑）得到 `appId`，并**校验 `config_type=file`**；若不是 file 型（如 kv 型），
   直接告知"该服务不是文件型服务"并停止（R-002）。
3. 参数齐了再进入对应意图的动作；能从上下文推断的（如上一步已拿到的 appId）不要重复问。

### 三类意图的最小引导

| 用户说 | 最少还需要问 | 拿齐后动作 |
|--------|-------------|-----------|
| 查询文件配置 / 看某文件服务配置 | bizId、服务名 | 列草稿态配置项或已发布配置项，展示 sign / path / name / byte_size |
| 文件配置项新增 / 更新 / 删除（仅元数据） | bizId、服务名、path/name（改/增再要**已上传内容的 sign**；删要 id） | 增删改配置项（草稿态）；完成后**主动追问"是否现在生成版本并发布？"**，同意再走发布 |
| 文件配置发布 / 发版 | bizId、服务名 | 有草稿改动 → `Config_CreateRelease` 拿 releaseId；或从 `Config_ListReleases` 选已有版本，再 `Config_Publish` |

### 写操作执行前必须二次确认

对 **发布（`Config_Publish`）**、**覆盖式批量导入（`Config_BatchUpsertConfigItems` 且 `replaceAll=true`）**、
**删除配置项** 这类会影响客户端生效内容或清空数据的操作，执行前先**展示将影响的内容**
（如将发布的版本、将被覆盖/删除的配置项列表）并等用户确认后再调用；用户未确认不执行。

## ⚠️ 核心规则

1. **文件配置项增删改都是草稿态**，客户端不可见；必须"生成版本 + 发布"后才生效（R-001）。
2. **文件型操作只适用于 `config_type=file` 的 app**；对 KV 型 app 操作文件接口会报错，务必先校验（R-002）。
3. **引用的内容 sign 必须是已上传内容**：创建/更新配置项引用的 sign（SHA256）须先由 UI/SDK 上传；
   未上传会报"内容未上传"，此时引导用户去 UI/SDK 上传，**不产生指向空对象的引用**（R-004）。
4. **`Config_Publish` 只发布已存在的 releaseId**，不会从草稿自动生成版本。
5. **灰度字段在 `Config_Publish` 上**（`grayPublishMode` / `groups` / `labels`）；`groups` 有值时
   `all` 必须为 `false`（R-003）。
6. **本期不含内容上传**：二进制文件上传走 UI/SDK；AI 只做元数据编排与 sign 引用。

## 领域模型速览（F-001）

```
biz（业务） → app（服务，config_type=file） → config_item（草稿态配置项：sign + 元数据） → release（不可变版本快照）
                                                    │
                          content（内容对象，按 sign 标识，已上传，只读引用）
```

- 文件型配置项由「内容 sign（SHA256）+ 元数据（path / name / byte_size / 权限等）」组织，
  **区别于 KV 型的 key / kvType / value**。
- config_item 每次增删改只改草稿区，不影响已发布内容；release 是一次生成后不可变的快照。
- content 是对象存储里以 sign 标识的已上传文件内容，本期**只读引用**，不上传、不透传字节。

## 端到端调用编排（F-002 ~ F-006）

标准闭环链路：

1. **定位服务** → 拿 `appId`
   - 已知业务：`Config_ListAppsBySpaceRest`（按 bizId 列 app）
   - 已知服务名：`Config_GetAppByName`
   - 从返回结果的 `config_type` 字段辨别 **file 型** app，非 file 型直接停止（R-002）
2. **查询现状**（F-001）
   - 草稿态配置项列表 / 详情、已发布配置项列表（读 sign / path / name / byte_size）
   - 删除或更新前先查到目标配置项的 `id`
3. **改配置项元数据（草稿态）**（F-002）
   - 单条：创建 / 更新 / 删除文件配置项（引用**已上传**内容 sign）
   - 批量：`Config_BatchUpsertConfigItems`（`replaceAll=true` 会先清空草稿区再写入）
   - 引用 sign 未上传 → 报"内容未上传"，引导走 UI/SDK（R-004）
4. **生成版本**（F-003）
   - `Config_CreateRelease` → 产出 `releaseId`
5. **发布**（F-004 全量 / F-005 灰度）
   - `Config_Publish`：发布已有 `releaseId`；`all=true` 全量，或灰度（见下）
6. **验证**（F-006，本期核心新增）
   - 用**文件下载 URL 工具**对目标内容 sign 取临时预签名下载 URL（响应只含 `download_url` +
     `expire_seconds`，**不含文件字节**），再由用户用该 URL 直连存储下载校验，避免大文件穿透管理面/网关。
   - 也可用已发布配置项列表确认元数据已生效。

### 全量 vs 灰度发布

| 场景 | 推荐工具 | 关键参数 |
|------|---------|---------|
| 生成版本并全量发布 | 先 `Config_CreateRelease` 拿 `releaseId`，再 `Config_Publish` | `all=true` |
| 发布某个已有版本（全量） | `Config_Publish` | `releaseId` + `all=true` |
| 灰度发布 | 先 `Config_CreateRelease` 拿 `releaseId`，再 `Config_Publish` | `all=false` + `grayPublishMode`（`publish_by_groups`/`publish_by_labels`）+ `groups` 或 `labels` |

> 注意：灰度相关字段（`grayPublishMode` / `groups` / `labels` / `groupName`）在 `Config_Publish` 的 body 上；
> `groups`（分组 ID 列表）有值时 `all` 必须为 `false`（R-003）。

## 参数获取

- `bizId`：来自蓝鲸平台上下文（CMDB 业务 / 空间）或请求头 `X-Bkapi-Biz-Id`；MCP **不提供列 biz 的工具**，需由用户/上下文给出。
- `appId`：通过 `Config_ListAppsBySpaceRest`（按 bizId 列 app）或 `Config_GetAppByName`（已知服务名）获取。
- **辨别 file 型 app**：从上述工具返回结果里读 `config_type` 字段，取 `config_type=file` 的 app。
- 配置项 `id`：删除/更新前用配置项列表查询获取（文件配置项按 id 定位，不是按 path）。
- 内容 `sign`（SHA256，64 位十六进制）：来自此前 UI/SDK 上传该文件后返回的内容标识；本 skill 不负责上传，
  只引用已有 sign。
- `releaseId`：由 `Config_CreateRelease` 返回，或从 `Config_ListReleases` 中选未废弃的版本。

## 报错 → 原因 → 处置

| 报错关键字 | 原因 | 处置 |
|-----------|------|------|
| `内容未上传` / `file content not uploaded` / `file content not found` | 引用的 sign 尚未上传到对象存储 | 引导用户先经 **UI/SDK 上传**该文件拿到 sign，再引用；不要用未上传的 sign 生成版本/取下载 URL |
| `not a file type service` / 服务类型不符 | 对非 file 型 app 操作文件接口 | 确认目标 app 的 `config_type=file`（R-002） |
| `already exists` / 路径重复 | 同一 app 内 config_item 路径冲突 | 改用更新，或换 path/name |
| `there is a release in publishing currently` | 已有版本正在上线 | 等待当前上线完成后再发布 |
| `release ... is deprecated` | 目标版本已废弃 | 换未废弃版本，或重新 `CreateRelease` |
| `exceeded the limit` | 配置项数超过上限 | 清理无用配置项后重试 |
| 鉴权失败 / 无权限 | 未通过业务/服务（内容）鉴权 | 确认对该 biz/app 有权限；鉴权失败不会返回下载 URL（不泄露） |

## 场景化示例

以下为调用序列示意（`bizId` / `appId` / `sign` / `releaseId` 用占位符，实际以获取到的值为准）。
工具名以当前 MCP 工具集实际暴露的为准；文件配置项与下载 URL 工具须已在网关注册。

### 1) 新增一个文件配置项并全量发布，再用下载 URL 验证

```
Config_ListAppsBySpaceRest {bizId} → 从结果中找 config_type=file 的目标 app，取其 appId
创建文件配置项 {bizId, appId} body: {path:"/etc/app", name:"app.conf", sign:"<已上传内容的 sha256>", byteSize:..., 权限...}
Config_CreateRelease {bizId, appId} → releaseId          // 生成版本
Config_Publish {bizId, appId, releaseId} body: {all:true} // 全量发布
下载URL工具 {bizId, appId, sign} → {download_url, expire_seconds:3600}   // 验证: 只拿 URL, 不透传字节
// 由用户用 download_url 直连存储下载校验文件内容
```

### 2) 批量覆盖文件配置项（replaceAll）并发布

```
Config_BatchUpsertConfigItems {bizId, appId} body: {replaceAll:true, items:[{path,name,sign,byteSize,...}, ...]}
Config_CreateRelease {bizId, appId} body: {name:"v-import-1"} → releaseId
Config_Publish {bizId, appId, releaseId} body: {all:true}
```

### 3) 删除一个文件配置项（草稿态）后发布

```
列出文件配置项 {bizId, appId} → 找到目标配置项的 id
删除文件配置项 {bizId, appId, id}                 // 草稿态
Config_CreateRelease {bizId, appId} → releaseId
Config_Publish {bizId, appId, releaseId} body: {all:true}  // 发布后才对客户端生效
```

### 4) 灰度发布到指定分组

```
Config_CreateRelease {bizId, appId} body: {name:"v-gray-1"} → releaseId
Config_Publish {bizId, appId, releaseId} body: {all:false, grayPublishMode:"publish_by_groups", groups:[<groupId>, ...]}
// 或按 labels：grayPublishMode:"publish_by_labels", labels:[{...}], 可选 groupName
```

### 5) 引用的内容未上传的处置

```
创建/更新文件配置项 {..., sign:"<未上传的 sha256>"} → 报"内容未上传"
→ 告知用户：请先经 UI/SDK 上传该文件得到 sign，再回来引用；本 AI 不负责二进制上传
```

## 文件型 vs KV 型差异（速查）

| 维度 | 文件型（本 skill） | KV 型（见 bscp-kv-config） |
|------|------------------|--------------------------|
| 配置对象 | config_item：sign（SHA256）+ 元数据（path/name/byte_size/权限） | kv：key / kvType / value |
| 类型校验 | 操作前校验 `config_type=file` | 操作前校验 `config_type=kv` |
| 内容来源 | 引用**已上传**内容 sign（本期不含上传） | 直接写 value |
| 发布后验证 | 用**下载 URL 接口**取临时 URL 直连存储校验（不透传字节） | `Config_ListReleasedKvs` 直读已发布值 |
| 共同闭环 | 草稿态增删改 → 生成版本 → 发布后生效 | 同左 |

## 说明

本文的领域约束与操作规范可能随 bscp 版本演进而变化。实际以工具调用的返回结果和报错信息为准；
遇到与本文不一致的情况，按报错对照表处置或咨询 bscp 平台。
