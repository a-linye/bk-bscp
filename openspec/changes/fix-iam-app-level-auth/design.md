## Context

BSCP 的 IAM 鉴权分为 HTTP 层中间件和 gRPC handler 层两道防线：

- **HTTP 层**（api-server）：`UnifiedAuthentication` 识别登录态，`BizVerified` 校验业务存在性，`AppVerified` 仅填充 `kit.AppID`/`SpaceID`——三者均不执行 IAM 资源实例授权。
- **gRPC handler 层**（config-server）：各 RPC handler 通过 `s.authorizer.Authorize(grpcKit, res...)` 手动构造 `ResourceAttribute` 列表进行鉴权。正确写法需同时包含 `meta.Biz` + `meta.App`（附 `ResourceID`），但部分 handler 仅写了 `meta.Biz`。
- **下游服务**（data-service / cache-service）：完全信任上游已鉴权，不做二次校验。

安全审计（`docs/security-audit-iam-app-level-auth.md`）发现 24+ 个接口存在 App 级鉴权缺失，涵盖 KV 配置、配置项、Hook、分组、导入导出、模板等多个模块。

## Goals / Non-Goals

**Goals:**

- 消除所有已识别的 Application 级水平越权漏洞
- 为 api-server 的 3 个裸 handler 补充入口鉴权（双层防御）
- 修复间接信息泄露接口（后置校验拦截）
- 为模板类接口叠加 App 级校验
- 保持与仓库现有鉴权模式完全一致，不引入新抽象

**Non-Goals:**

- 不引入统一 gRPC 鉴权拦截器（属于长期治理）
- 不升级 `AppVerified` 中间件为 IAM 实例校验（属于长期治理）
- 不修改 `Approve` 的审批通过/驳回鉴权逻辑（依赖 ITSM 工单权限控制）
- 不处理 `ApprovalCallback`（ITSM 回调依赖 callback_token 校验）
- 不处理 `ListAudits`（有独立的 `meta.Audit` + `View` 权限模型）

## Decisions

### D1：逐接口修补而非统一拦截器

**选择**：在每个缺失鉴权的 handler 内部，按现有正确写法（如 `UndoKv`、`GetApp`）手动补充 `meta.App` + `ResourceID`。

**替代方案**：gRPC 拦截器统一校验。

**理由**：安全漏洞修复优先速度和确定性。逐接口修补完全复用现有模式，每个修复点可独立 review 和测试，风险最低。拦截器方案需要处理 action 映射、特殊参数等复杂性，不适合紧急修复。

### D2：Action 映射规则

| 操作性质 | Action | 依据 |
|---|---|---|
| 查询/导出/统计/读取 | `meta.View` | 与 `GetApp`、`ListReleasedKvs`、`ListConfigItems` 一致 |
| 创建/修改/删除/导入 | `meta.Update` | 与 `UndoKv`、`BatchDeleteKv`、`ImportKvs`、`CreateConfigItem` 一致 |

### D3：api-server 双层防御

**选择**：在 `ConfigFileImport`、`ConfigFileExport`、`kvService.Export` 的 HTTP handler 入口处新增 `authorizer.Authorize` 调用，同时修复 config-server 侧下游 RPC 的鉴权缺口。

**理由**：即使某些下游 RPC 已有 App 校验（如 `ListConfigItems`），上游也应做一次校验，防止未来下游调整时产生遗漏。三个 handler 结构体已持有 `authorizer` 字段，改动极小。

### D4：GetAppByName 先查询再鉴权

**选择**：先通过 name 查到 app 记录拿到 ID，再用该 ID 做 App View 校验。校验失败返回权限错误。

**理由**：`GetAppByName` 的请求参数是 `app_name` 而非 `app_id`，无法直接构造 `ResourceID`。先查后鉴虽然会暴露"应用是否存在"的信息，但该信息本身属于业务级范畴（已有 Biz 权限即可感知），安全影响可接受。

### D5：间接泄露接口采用接口级拦截

**选择**：对 `ListGroupReleasedApps`、`ListHookReferences`、`ListHookRevisionReferences` 的响应中所有 AppId 做批量 App View 校验，任一无权限则拒绝整个请求。

**替代方案**：响应过滤（仅返回有权限的记录）。

**理由**：接口级拦截实现简单、语义清晰。使用已有的 `Authorize` 方法传入多个 `ResourceAttribute` 即可批量校验。

### D6：模板类接口叠加 App 校验

对参数中携带 AppId/AppIds/BoundApps 的模板接口，在现有模板权限校验基础上叠加 App 级校验。批量 AppId 场景使用与 D5 相同的批量校验方式。

## Risks / Trade-offs

- **[行为变更]** 原本无 App 权限也能访问的接口将返回 403。→ 缓解：属于修正不正确的行为，上线前需通知用户团队确认权限配置。
- **[性能开销]** 批量 AppId 校验（间接泄露/模板接口）会增加 IAM 调用次数。→ 缓解：`AuthorizeBatch` 支持批量校验，单次 RPC 即可完成；涉及的 AppId 数量通常较少。
- **[GetAppByName 信息泄露]** 先查询再鉴权会暴露应用是否存在。→ 缓解：调用者已有 Biz 权限，业务内应用存在性非敏感信息。
- **[接口级拦截的可用性]** 对间接泄露接口采用全量拦截，用户可能因对某个关联应用无权限而无法查看任何引用关系。→ 缓解：这是安全优先的选择，可在后续版本中根据产品需求调整为响应过滤。

## Implementation Deviations

- **ManageConfigKV 跳过**：`ManageConfigKVReq` 无 `BizId`/`AppId` 字段，为系统级配置管理 API（操作通用 configs 表），App 级校验不适用。仅有 `UnifiedAuthentication` 登录态校验。
- **配置模板相关接口暂缓（任务组 6 + 5.3）**：CreateTemplateSet/UpdateTemplateSet（BoundApps）、ListTmplSetsOfBiz、BatchUpdateTemplatePermissions（AppIds）、CheckTemplateSetReferencesApps、ListAppTemplateSets — 如果对配置模板查看增加 App 权限校验，会导致用户进入页面后出现大量 403 错误提示，体验不佳。暂不处理，待产品层面确定降级方案后再添加。
- **间接泄露接口后置校验暂缓（任务 3.3-3.5）**：ListGroupReleasedApps、ListHookReferences、ListHookRevisionReferences — 后置校验会在用户对任一关联 App 无权限时拒绝整个请求，同样导致页面报错。暂不处理，待确定降级方案（如响应过滤而非全量拒绝）后再添加。
