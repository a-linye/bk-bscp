## ADDED Requirements

### Requirement: KV 操作须校验 App 级权限

所有 KV 配置操作接口（CreateKv、UpdateKv、ListKvs、DeleteKv、BatchUpsertKvs、UnDeleteKv、FindNearExpiryCertKvs）在执行前 SHALL 通过 IAM 校验用户对目标 AppId 的权限。写操作使用 `meta.Update`，读操作使用 `meta.View`。

#### Scenario: 无 App 权限用户尝试创建 KV
- **WHEN** 用户有 Biz 权限但无目标 App 的 Update 权限，调用 CreateKv
- **THEN** 系统 SHALL 返回权限错误，不执行创建

#### Scenario: 有 App 权限用户正常读取 KV 列表
- **WHEN** 用户有 Biz 权限且有目标 App 的 View 权限，调用 ListKvs
- **THEN** 系统 SHALL 正常返回 KV 列表

#### Scenario: 无 App 权限用户尝试删除 KV
- **WHEN** 用户有 Biz 权限但无目标 App 的 Update 权限，调用 DeleteKv
- **THEN** 系统 SHALL 返回权限错误，不执行删除

### Requirement: 配置项统计和查询须校验 App 级权限

ListConfigItemCount、ListConfigItemByTuple、GetTemplateAndNonTemplateCICount、CompareConfigItemConflicts 接口 SHALL 校验用户对目标 AppId 的 View 权限。CompareConfigItemConflicts 须同时校验 `req.AppId` 和 `req.OtherAppId`。

#### Scenario: 无 App 权限用户尝试查询配置项统计
- **WHEN** 用户有 Biz 权限但无目标 App 的 View 权限，调用 ListConfigItemCount
- **THEN** 系统 SHALL 返回权限错误

#### Scenario: 跨服务冲突比较须校验两个 App
- **WHEN** 用户有 AppId 的 View 权限但无 OtherAppId 的 View 权限，调用 CompareConfigItemConflicts
- **THEN** 系统 SHALL 返回权限错误

#### Scenario: 用户对两个 App 均有权限时正常比较
- **WHEN** 用户有 AppId 和 OtherAppId 的 View 权限，调用 CompareConfigItemConflicts
- **THEN** 系统 SHALL 正常返回冲突比较结果

### Requirement: Hook 内容读取须校验 App 级权限

GetReleaseHook 接口 SHALL 校验用户对目标 AppId 的 View 权限。

#### Scenario: 无 App 权限用户尝试读取 Hook 内容
- **WHEN** 用户有 Biz 权限但无目标 App 的 View 权限，调用 GetReleaseHook
- **THEN** 系统 SHALL 返回权限错误

### Requirement: 分组关系查询须校验 App 级权限

ListAppGroups 接口 SHALL 校验用户对目标 AppId 的 View 权限。

#### Scenario: 无 App 权限用户尝试查询服务分组
- **WHEN** 用户有 Biz 权限但无目标 App 的 View 权限，调用 ListAppGroups
- **THEN** 系统 SHALL 返回权限错误

### Requirement: 间接泄露接口须拦截无权限请求

ListGroupReleasedApps、ListHookReferences、ListHookRevisionReferences 接口在返回结果前 SHALL 对响应中所有涉及的 AppId 做批量 App View 校验。任一 AppId 无权限则拒绝整个请求。

#### Scenario: 响应中包含用户无权限的 App
- **WHEN** 用户调用 ListGroupReleasedApps，结果中包含用户无 View 权限的 AppId
- **THEN** 系统 SHALL 返回权限错误，不返回任何数据

#### Scenario: 响应中所有 App 用户均有权限
- **WHEN** 用户调用 ListHookReferences，结果中所有 AppId 用户均有 View 权限
- **THEN** 系统 SHALL 正常返回完整引用列表

### Requirement: 配置文件导入导出须校验 App 级权限

api-server 的 ConfigFileImport、ConfigFileExport、kvService.Export 三个 HTTP handler SHALL 在入口处校验用户对目标 AppId 的权限。Import 使用 `meta.Update`，Export 使用 `meta.View`。

#### Scenario: 无 App 权限用户尝试导入配置文件
- **WHEN** 用户有 Biz 权限但无目标 App 的 Update 权限，调用 ConfigFileImport
- **THEN** 系统 SHALL 返回权限错误，不执行导入

#### Scenario: 无 App 权限用户尝试导出配置文件
- **WHEN** 用户有 Biz 权限但无目标 App 的 View 权限，调用 ConfigFileExport
- **THEN** 系统 SHALL 返回权限错误，不执行导出

### Requirement: GetAppByName 须校验 App 级权限

GetAppByName 接口 SHALL 在查询到应用记录后，使用返回的 AppId 校验用户的 App View 权限。

#### Scenario: 无 App 权限用户按名称查询应用
- **WHEN** 用户有 Biz 权限但无目标 App 的 View 权限，调用 GetAppByName
- **THEN** 系统 SHALL 返回权限错误

#### Scenario: 应用不存在时正常返回错误
- **WHEN** 用户调用 GetAppByName 查询不存在的应用名
- **THEN** 系统 SHALL 返回应用不存在的错误（鉴权不介入）

### Requirement: ManageConfigKV 须校验 App 级权限

ManageConfigKV 接口 SHALL 校验用户对目标 AppId 的权限。

#### Scenario: 无权限用户尝试管理配置 KV
- **WHEN** 用户无 Biz 或 App 权限，调用 ManageConfigKV
- **THEN** 系统 SHALL 返回权限错误

### Requirement: ListAppTemplateSets 须包含 ResourceID

ListAppTemplateSets 的 IAM 校验中 `meta.App` + `View` 的 `ResourceID` SHALL 设置为 `req.AppId`。

#### Scenario: ResourceID 正确绑定
- **WHEN** 用户无目标 App 的 View 权限，调用 ListAppTemplateSets
- **THEN** 系统 SHALL 返回权限错误（因 ResourceID 正确传入，IAM 能校验到具体应用实例）

### Requirement: 模板类接口须叠加 App 级校验

CreateTemplateSet、UpdateTemplateSet（对 BoundApps）、ListTmplSetsOfBiz（对 AppId 过滤条件）、BatchUpdateTemplatePermissions（对 AppIds）、CheckTemplateSetReferencesApps（对响应中的 AppId）SHALL 校验用户对涉及的每个 AppId 的 View 权限。

#### Scenario: 模板套餐绑定无权限的应用
- **WHEN** 用户调用 CreateTemplateSet，BoundApps 中包含用户无 View 权限的 AppId
- **THEN** 系统 SHALL 返回权限错误

#### Scenario: 批量更新模板权限时涉及无权限应用
- **WHEN** 用户调用 BatchUpdateTemplatePermissions，AppIds 中包含用户无 View 权限的 AppId
- **THEN** 系统 SHALL 返回权限错误

#### Scenario: 查询模板套餐引用关系时涉及无权限应用
- **WHEN** 用户调用 CheckTemplateSetReferencesApps，响应中包含用户无 View 权限的 AppId
- **THEN** 系统 SHALL 返回权限错误
