## Why

BSCP 管理端多条挂在 `/api/v1/config/biz/{biz_id}/apps/{app_id}` 路径下的服务级接口，虽然 URL 带有 `app_id`，但实际只做了 Biz 级 IAM 校验，未对目标服务对象执行 Application 级授权校验。已登录用户若可进入目标业务上下文但没有目标服务权限，仍可跨服务读取、写入、删除配置数据，属于水平越权漏洞。详见 `docs/security-audit-iam-app-level-auth.md`。

## What Changes

- 为 config-server 中 14 个仅有 Biz 级校验的 handler 补充 `meta.App` + `ResourceID` 维度的 IAM 授权校验
- 为 api-server 中 3 个无鉴权的 HTTP handler（ConfigFileImport/Export、kvService.Export）在入口处添加 Authorize 调用
- 恢复 `GetAppByName` 被注释的 IAM 校验，采用先查询再鉴权方式
- 为 `ManageConfigKV` 添加完整鉴权逻辑
- 修复 `ListAppTemplateSets` 中 `meta.App` 缺少 `ResourceID` 的问题
- 为 `ListGroupReleasedApps`、`ListHookReferences`、`ListHookRevisionReferences` 添加后置 App 权限校验，拦截无权限请求
- 为模板类接口（`CreateTemplateSet`/`UpdateTemplateSet`/`ListTmplSetsOfBiz`/`BatchUpdateTemplatePermissions`/`CheckTemplateSetReferencesApps`）叠加 App 级校验
- `Approve` 的审批通过/驳回场景保持现状，依赖 ITSM 工单权限控制

## Capabilities

### New Capabilities

- `iam-app-auth`: IAM Application 级授权校验修复，覆盖 config-server 和 api-server 中所有缺失 App 级鉴权的接口

### Modified Capabilities

## Impact

- **config-server**：`kv.go`、`config_item.go`、`hook.go`、`hook_revision.go`、`group.go`、`app.go`、`template_set.go`、`template.go`、`template_binding_relation.go`、`process_config_view.go` 中的 handler 函数
- **api-server**：`config_import.go`、`config_export.go`、`released_kv.go` 中的 HTTP handler
- **API 行为变更**：原本无 App 权限也能访问的接口将返回 403，对无权限用户存在 **BREAKING** 行为变更
- **依赖**：无新增依赖，完全复用现有 `pkg/iam/meta` 和 `internal/iam/auth` 组件
