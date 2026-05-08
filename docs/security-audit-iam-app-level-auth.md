# BSCP IAM Application 级授权缺失安全审计报告

> 审计时间：2026-05-08  
> 审计范围：bk-bscp 全项目（config-server / api-server / data-service / feed-server / cache-service）  
> 审计目标：排查 `/api/v1/config/biz/{biz_id}/apps/{app_id}` 路径下的服务级接口是否对目标服务对象执行了 IAM Application 级授权校验  
> 审计修订：经交叉对比校验，补充配置项侧、分组/发布关系侧、Hook 引用侧遗漏，并调整模板/审批类接口分类

---

## 一、问题概述

BSCP 管理端多条挂在 `/api/v1/config/biz/{biz_id}/apps/{app_id}` 路径下的服务级接口，虽然表面上带有 `app_id`，但实际只做了登录态和 `biz_id` 存在性相关处理，**未对目标服务对象执行 IAM 的 Application 级授权校验**。

已登录用户若可以进入目标业务上下文但没有目标服务权限，仍可：

- 按服务名获取目标服务元数据
- 跨服务读取 KV 配置
- 跨服务读取配置项元数据和统计信息
- 获取目标服务已上线版本的 release_id，并读取该版本的前后置 Hook 内容
- 通过 Hook 引用关系、分组发布关系泄露服务/版本信息
- 跨服务创建/修改/删除 KV 数据
- 导入/导出配置文件

属于**水平越权漏洞**，影响面覆盖读、写、删三类操作。

---

## 二、鉴权架构分析

### 2.1 整体链路

```
HTTP 请求 → api-server → gRPC → config-server → data-service
```

### 2.2 HTTP 层中间件（api-server）

| 中间件 | 作用 | 是否做 IAM |
|--------|------|-----------|
| `UnifiedAuthentication` | 识别登录态（JWT/Cookie），构造 kit | 否 |
| `BizVerified` | 校验 `biz_id` 对应的 CMDB 业务是否存在 | **否**（仅存在性检查） |
| `AppVerified` | 通过 `QuerySpaceByAppID` 查询应用是否存在，填充 `kit.AppID`/`SpaceID` | **否**（仅存在性+上下文填充） |

关键代码 — `AppVerified` 中间件（`internal/iam/auth/middleware.go:260-288`）：

```go
func (a authorizer) AppVerified(next http.Handler) http.Handler {
    fn := func(w http.ResponseWriter, r *http.Request) {
        kt := kit.MustGetKit(r.Context())
        appIDStr := chi.URLParam(r, "app_id")
        // ...解析 app_id...
        space, err := a.authClient.QuerySpaceByAppID(kt.RpcCtx(),
            &pbas.QuerySpaceByAppIDReq{AppId: uint32(appID)})
        // ...仅填充 kit，不做 IAM 鉴权...
        kt.AppID = uint32(appID)
        kt.SpaceID = space.SpaceId
        kt.SpaceTypeID = space.SpaceTypeId
    }
    return http.HandlerFunc(fn)
}
```

`QuerySpaceByAppID` 实现（`cmd/auth-server/service/service.go:591-609`）仅调用 `GetAppByID`，不做任何 IAM 校验。

### 2.3 gRPC 层（config-server）

IAM 校验分散在各 RPC handler 内部，通过 `s.authorizer.Authorize(grpcKit, res...)` 手动调用。

**正确写法**（同时校验 Biz + App）：

```go
res := []*meta.ResourceAttribute{
    {Basic: meta.Basic{Type: meta.Biz, Action: meta.FindBusinessResource}, BizID: req.BizId},
    {Basic: meta.Basic{Type: meta.App, Action: meta.View, ResourceID: req.AppId}, BizID: req.BizId},
}
```

**问题写法**（仅校验 Biz）：

```go
res := []*meta.ResourceAttribute{
    {Basic: meta.Basic{Type: meta.Biz, Action: meta.FindBusinessResource}, BizID: req.BizId},
}
// 缺少 meta.App + ResourceID，后续却使用了 req.AppId
```

### 2.4 下游服务

| 服务 | IAM 校验 | 说明 |
|------|---------|------|
| data-service | **无** | 完全信任上游 config-server 已鉴权 |
| cache-service | **无** | 依赖部署网络隔离 |
| feed-server | Sidecar + Credential 模型 | 非控制台用户 IAM 体系 |

---

## 三、问题接口清单

### 3.1 类型 A：仅有 Biz 级校验但使用了 AppId

这些接口调用了 `s.authorizer.Authorize`，但 `ResourceAttribute` 中仅包含 `meta.Biz`，未包含 `meta.App` + `ResourceID`。

#### 3.1.1 KV 配置操作（`cmd/config-server/service/kv.go`）

| 接口 | 行号 | HTTP 方法 & 路径 | 操作类型 |
|------|------|-----------------|---------|
| `CreateKv` | 50 | `POST /api/v1/config/biz/{biz_id}/apps/{app_id}/kvs` | **写入** |
| `UpdateKv` | 93 | `PUT /api/v1/config/biz/{biz_id}/apps/{app_id}/kvs/{key}` | **写入** |
| `ListKvs` | 131 | `POST /api/v1/config/biz/{biz_id}/apps/{app_id}/kvs/list` | 读取 |
| `DeleteKv` | 192 | `DELETE /api/v1/config/biz/{biz_id}/apps/{app_id}/kvs/{id}` | **删除** |
| `BatchUpsertKvs` | 299 | `PUT /api/v1/config/biz/{biz_id}/apps/{app_id}/kvs` | **批量写入** |
| `UnDeleteKv` | 349 | `POST /api/v1/config/biz/{biz_id}/apps/{app_id}/kvs/{key}/undelete` | **写入** |
| `FindNearExpiryCertKvs` | 874 | `GET /api/v1/config/biz/{biz_id}/apps/{app_id}/kvs/near_certificate` | 读取 |

> **对比**：同文件中的 `UndoKv`(375)、`BatchDeleteKv`(222)、`CompareKvConflicts`(401)、`ImportKvs`(491)、`BatchUnDeleteKv`(795) **正确包含** `meta.App` + `ResourceID`。

#### 3.1.2 配置项操作（`cmd/config-server/service/config_item.go`）

| 接口 | 行号 | 使用的 AppId 字段 | 操作类型 |
|------|------|------------------|---------|
| `ListConfigItemCount` | 541 | `req.AppId` — 批量按 app_id 统计配置项数量 | 读取 |
| `ListConfigItemByTuple` | 569 | `req.AppId` — 按 app_id 查询配置项 tuple | 读取 |
| `CompareConfigItemConflicts` | 656 | `req.AppId` + `req.OtherAppId` — 跨服务冲突比较，两个 app_id 均未校验 | 读取（跨服务） |
| `GetTemplateAndNonTemplateCICount` | 723 | `req.AppId` — 按服务统计模板/非模板配置项数量 | 读取 |

> **对比**：同文件中的 `CreateConfigItem`(42)、`BatchUpsertConfigItems`(101)、`UpdateConfigItem`(171)、`DeleteConfigItem`(275)、`GetConfigItem`(372)、`ListConfigItems`(466) 等 **正确包含** `meta.App` + `ResourceID`。  
> **特别注意**：`CompareConfigItemConflicts` 不仅当前 `req.AppId` 缺少 App 级校验，`req.OtherAppId`（跨服务方）也完全未做 App View 授权，风险更高。

#### 3.1.3 Hook 操作（`cmd/config-server/service/hook.go`、`hook_revision.go`）

| 接口 | 文件 | 行号 | 说明 | 操作类型 |
|------|------|------|------|---------|
| `GetReleaseHook` | `hook.go` | 408 | 按 app_id + release_id 读取版本前后置 Hook 内容 | 读取 Hook Content |
| `ListHookReferences` | `hook.go` | 358 | 返回 Hook 被哪些服务/版本引用（含 AppId/AppName/ReleaseId/ReleaseName） | 信息泄露 |
| `ListHookRevisionReferences` | `hook_revision.go` | 228 | 返回 Hook 版本被哪些服务/版本引用（含 AppId/AppName/ReleaseId/ReleaseName） | 信息泄露 |

> **对比**：`config_hook.go` 中的 `UpdateConfigHook`(34) **正确包含** `meta.App` + `ResourceID`。  
> **说明**：`ListHookReferences` / `ListHookRevisionReferences` 虽然不在 `/apps/{app_id}` 路由下，但响应体包含 `AppId`/`AppName`/`ReleaseId`/`ReleaseName`，无应用权限用户可通过此接口泄露服务和版本关联信息。从泄露 release_id 的角度看与本问题相近，建议同步评估。

#### 3.1.4 分组/发布关系（`cmd/config-server/service/group.go`）

| 接口 | 行号 | 说明 | 操作类型 |
|------|------|------|---------|
| `ListAppGroups` | 313 | 按 app_id 获取服务关联分组及 release_id/release_name | 读取 |
| `ListGroupReleasedApps` | 352 | 按分组反查已上线服务，返回 AppId/AppName/ReleaseId/ReleaseName | 信息泄露 |

> **说明**：`ListGroupReleasedApps` 虽然入参是 `GroupId` 不是 `AppId`，但响应中返回完整的 `AppId`/`AppName`/`ReleaseId`/`ReleaseName` 列表，无应用级权限的用户可通过业务下分组遍历所有已上线服务及其版本信息。

---

### 3.2 类型 B：有 `meta.App` 但缺少 `ResourceID`

| 接口 | 文件 | 行号 | 说明 |
|------|------|------|------|
| `ListAppTemplateSets` | `template_set.go` | 180 | `{Type: meta.App, Action: meta.View}` 中 **ResourceID 为空**，IAM 无法校验到具体应用实例 |

---

### 3.3 类型 C：完全无 Authorize 调用

| 接口 | 文件 | 行号 | HTTP 路径 | 说明 |
|------|------|------|-----------|------|
| `GetAppByName` | `app.go` | 179 | `GET .../apps/query/name/{app_name}` | 整段 IAM 代码被注释，带 `TODO: 暂不鉴权` |
| `ManageConfigKV` | `process_config_view.go` | 25 | `/api/v1/config/manage_config_kv` | 无任何 Authorize 调用 |

---

### 3.4 类型 D：条件性鉴权缺失

| 接口 | 文件 | 行号 | 说明 |
|------|------|------|------|
| `Approve` | `publish.go` | 221 | 当 `PublishStatus` 为"审批通过"或"驳回"时 **跳过 Authorize**，仅"撤销/上线"才校验 App 级权限 |

---

### 3.5 类型 E：api-server 自定义 Handler 无鉴权

以下路由 URL 含 `app_id`，但仅配了 `BizVerified`（无 `AppVerified`），且 **handler 内部未调用 `Authorize`**：

| HTTP 路径 | handler 文件 | handler 方法 | 操作类型 |
|-----------|-------------|-------------|---------|
| `POST /api/v1/config/biz/{biz_id}/apps/{app_id}/config_item/import/{filename}` | `config_import.go` | `ConfigFileImport` | **写入**（文件上传+repo 存储） |
| `GET /api/v1/config/biz/{biz_id}/apps/{app_id}/releases/{release_id}/config_item/export` | `config_export.go` | `ConfigFileExport` | 读取（文件下载） |
| `GET /api/v1/biz/{biz_id}/apps/{app_id}/releases/{release_id}/kvs/export` | `released_kv.go` | `kvService.Export` | 读取（KV 导出） |

> 注意：`ConfigFileExport` 和 `ConfigFileImport` 的结构体中持有 `authorizer` 字段，但 handler 中**从未调用**。

---

### 3.6 相关但不直接归入本次 Application 授权缺失的接口

以下接口有关联但建议独立评估，不与本次 App 越权补丁混在一起：

| 接口 | 文件 | 行号 | 说明 | 建议 |
|------|------|------|------|------|
| `ApprovalCallback` | `release.go` | 302 | 路由有 app_id/release_id，但为 ITSM 审批回调路径，无 IAM。安全性依赖 `callback_token` 强校验 | 确认 callback_token 校验是否充分 |
| `ListAudits` | `audit.go` | 29 | 可按 app_id 过滤，但有独立的 `meta.Audit` + `View` 权限模型 | 确认是否需要叠加 App View |
| `CreateTemplateSet` | `template_set.go` | 60 | `req.BoundApps` 含应用 ID 列表，但资源主体是模板套餐 | 建议另开模板权限模型审计 |
| `UpdateTemplateSet` | `template_set.go` | 126 | `req.BoundApps` 含应用 ID 列表，同上 | 建议另开模板权限模型审计 |
| `ListTmplSetsOfBiz` | `template_set.go` | 256 | `req.AppId` 作为过滤条件，资源主体偏模板空间 | 建议另开模板权限模型审计 |
| `BatchUpdateTemplatePermissions` | `template.go` | 579 | `req.AppIds` 含应用 ID 列表，资源主体偏模板 | 建议另开模板权限模型审计 |
| `CheckTemplateSetReferencesApps` | `template_binding_relation.go` | 539 | 响应中返回 AppId/AppName，资源主体偏模板套餐 | 建议另开模板权限模型审计 |

---

## 四、影响面分析

### 4.1 按操作类型分类

| 操作类型 | 受影响接口数 | 代表接口 |
|---------|------------|---------|
| **写入/创建** | 7 | `CreateKv`, `UpdateKv`, `BatchUpsertKvs`, `UnDeleteKv`, `ConfigFileImport` |
| **删除** | 1 | `DeleteKv` |
| **读取/信息泄露** | 14 | `ListKvs`, `GetReleaseHook`, `ListAppGroups`, `GetAppByName`, `ConfigFileExport`, `kvService.Export`, `FindNearExpiryCertKvs`, `ListConfigItemByTuple`, `ListConfigItemCount`, `GetTemplateAndNonTemplateCICount`, `CompareConfigItemConflicts`, `ListGroupReleasedApps`, `ListHookReferences`, `ListHookRevisionReferences` |
| **权限绕过** | 2 | `Approve`（部分场景）, `ManageConfigKV` |

### 4.2 攻击场景

攻击前提：攻击者拥有**目标业务的访问权限**（可进入业务上下文），但**无目标服务的 Application 权限**。

| 场景 | 攻击路径 | 影响 |
|------|---------|------|
| 信息收集 | 调用 `GetAppByName` 枚举服务元数据（含 ConfigType/DataType/创建者等） | 信息泄露 |
| 跨服务读 KV | 调用 `ListKvs` 传入目标 `app_id` | 配置数据泄露（含敏感类 KV） |
| 跨服务写 KV | 调用 `CreateKv`/`UpdateKv`/`BatchUpsertKvs` | 篡改他人服务配置 |
| 读取 Hook 脚本 | 调用 `GetReleaseHook` | 泄露脚本内容（可能含凭据/内部逻辑） |
| 配置项信息泄露 | 调用 `ListConfigItemByTuple`/`ListConfigItemCount`/`GetTemplateAndNonTemplateCICount` | 泄露配置项元数据和统计信息 |
| 跨服务冲突比较 | 调用 `CompareConfigItemConflicts` 传入 `OtherAppId` | 读取另一服务的配置项列表和模板绑定关系 |
| 服务/版本关系泄露 | 调用 `ListGroupReleasedApps` / `ListHookReferences` / `ListHookRevisionReferences` | 泄露服务名、版本名、release_id 等关联信息 |
| 配置文件下载 | 调用 `ConfigFileExport` / `kvService.Export` | 批量导出他人服务配置 |
| 配置文件上传 | 调用 `ConfigFileImport` | 向他人服务注入配置文件 |

---

## 五、修复建议

### 5.1 短期修复（第一优先级 — 直接越权读写）

为所有**直接按 `req.AppId` 读写服务资源**的 handler，在 `Authorize` 的 `ResourceAttribute` 中补充 `meta.App` 维度：

```go
// 修复前
res := []*meta.ResourceAttribute{
    {Basic: meta.Basic{Type: meta.Biz, Action: meta.FindBusinessResource}, BizID: req.BizId},
}

// 修复后（读操作）
res := []*meta.ResourceAttribute{
    {Basic: meta.Basic{Type: meta.Biz, Action: meta.FindBusinessResource}, BizID: req.BizId},
    {Basic: meta.Basic{Type: meta.App, Action: meta.View, ResourceID: req.AppId}, BizID: req.BizId},
}

// 修复后（写操作）
res := []*meta.ResourceAttribute{
    {Basic: meta.Basic{Type: meta.Biz, Action: meta.FindBusinessResource}, BizID: req.BizId},
    {Basic: meta.Basic{Type: meta.App, Action: meta.Update, ResourceID: req.AppId}, BizID: req.BizId},
}
```

需修复的接口（按文件）：

**kv.go**：`CreateKv`、`UpdateKv`、`ListKvs`、`DeleteKv`、`BatchUpsertKvs`、`UnDeleteKv`、`FindNearExpiryCertKvs`

**config_item.go**：`ListConfigItemCount`、`ListConfigItemByTuple`、`GetTemplateAndNonTemplateCICount`、`CompareConfigItemConflicts`（注意需同时对 `req.AppId` 和 `req.OtherAppId` 校验）

**hook.go**：`GetReleaseHook`

**group.go**：`ListAppGroups`

**api-server**：`ConfigFileExport`、`ConfigFileImport`、`kvService.Export` — 在 handler 函数入口处添加 `authorizer.Authorize` 调用

### 5.2 第二优先级 — 间接信息泄露

| 接口 | 修复方式 |
|------|---------|
| `ListGroupReleasedApps` | 响应中返回的 AppId/AppName 需过滤用户无权限的服务，或对接口叠加 App 级校验 |
| `ListHookReferences` | 响应中返回的 AppId/AppName/ReleaseId 需过滤，或确认产品设计是否允许无服务权限用户查看引用关系 |
| `ListHookRevisionReferences` | 同上 |

### 5.3 第三优先级 — 其他鉴权缺失

1. **恢复 `GetAppByName` 的 IAM 校验**：取消注释并补充必要参数
2. **为 `ManageConfigKV` 添加鉴权逻辑**
3. **为 `ListAppTemplateSets` 补充 `ResourceID`**：`{Type: meta.App, Action: meta.View, ResourceID: req.AppId}`
4. **修复 `Approve` 在"审批通过/驳回"场景的鉴权缺失**
5. **确认 `ApprovalCallback` 的 `callback_token` 校验是否充分**

### 5.4 独立审计项

建议另开专项处理，不与本次 App 越权补丁混合：

- **模板权限模型审计**：`CreateTemplateSet`/`UpdateTemplateSet`/`ListTmplSetsOfBiz`/`BatchUpdateTemplatePermissions`/`CheckTemplateSetReferencesApps` 参数中携带应用信息，但资源主体偏模板/模板空间，需评估模板权限模型是否应叠加 App 级校验
- **审计日志权限**：`ListAudits` 可按 `app_id` 过滤，但有独立的 `meta.Audit` + `View` 权限模型，需确认是否需叠加 App View

### 5.5 长期治理

1. **引入统一 gRPC 拦截器**：在 config-server 的 gRPC 拦截器链中增加统一的 App 级 IAM 校验拦截器，避免依赖各 handler 手动添加
2. **增加 CI 静态扫描**：编写规则检查所有使用 `req.AppId` 的 handler 是否包含 `meta.App` 资源属性
3. **升级 `AppVerified` 中间件**：在 HTTP 层直接集成 IAM 应用实例权限校验，作为纵深防御
4. **data-service 增加防御性校验**：至少验证 `app_id` 归属于 `biz_id`，防止跨业务攻击

---

## 附录：正确实现的接口参考

以下接口**正确包含**了 `meta.App` + `ResourceID` 校验，可作为修复参考：

| 接口 | 文件 | 行号 | 资源属性 |
|------|------|------|---------|
| `GetApp` | `app.go` | 158 | `meta.App, View, ResourceID: req.AppId` |
| `UndoKv` | `kv.go` | 375 | `meta.App, Update, ResourceID: req.AppId` |
| `BatchDeleteKv` | `kv.go` | 222 | `meta.App, Update, ResourceID: req.AppId` |
| `ImportKvs` | `kv.go` | 491 | `meta.App, Update, ResourceID: req.AppId` |
| `GetReleasedKv` | `released_kv.go` | 34 | `meta.App, View, ResourceID: req.AppId` |
| `ListReleasedKvs` | `released_kv.go` | 67 | `meta.App, View, ResourceID: req.AppId` |
| `Publish` | `publish.go` | 38 | `meta.App, Publish, ResourceID: req.AppId` |
| `CreateRelease` | `release.go` | 34 | `meta.App, GenerateRelease, ResourceID: req.AppId` |
| `UpdateConfigHook` | `config_hook.go` | 34 | `meta.App, Update, ResourceID: req.AppId` |
| `CreateConfigItem` | `config_item.go` | 42 | `meta.App, Update, ResourceID: req.AppId` |
| `GetConfigItem` | `config_item.go` | 372 | `meta.App, View, ResourceID: req.AppId` |
| `ListConfigItems` | `config_item.go` | 466 | `meta.App, View, ResourceID: req.AppId` |
| `RemoveAppBoundTmplSet` | `config_item.go` | 759 | `meta.App, Update, ResourceID: req.AppId` |
