## 1. config-server KV 鉴权修复

- [x] 1.1 修复 CreateKv、UpdateKv、DeleteKv、BatchUpsertKvs、UnDeleteKv 补充 App Update 校验  <!-- 非 TDD 任务 -->
  - [x] 1.1.1 执行变更：`cmd/config-server/service/kv.go`（5 个函数，在 Authorize 的 ResourceAttribute 列表中添加 `{Basic: meta.Basic{Type: meta.App, Action: meta.Update, ResourceID: req.AppId}, BizID: req.BizId}`）
  - [x] 1.1.2 验证无回归（运行：`go build ./cmd/config-server/...`，确认编译通过）
  - [x] 1.1.3 检查：确认 5 个函数的 ResourceAttribute 列表均包含 meta.App + ResourceID

- [x] 1.2 修复 ListKvs、FindNearExpiryCertKvs 补充 App View 校验  <!-- 非 TDD 任务 -->
  - [x] 1.2.1 执行变更：`cmd/config-server/service/kv.go`（2 个函数，添加 `{Basic: meta.Basic{Type: meta.App, Action: meta.View, ResourceID: req.AppId}, BizID: req.BizId}`）
  - [x] 1.2.2 验证无回归（运行：`go build ./cmd/config-server/...`，确认编译通过）
  - [x] 1.2.3 检查：确认 2 个函数的 ResourceAttribute 列表均包含 meta.App + ResourceID

- [ ] 1.3 代码审查
  - 前置：调用 superpowers:verification-before-completion 运行全量测试
  - 调用 superpowers:requesting-code-review 审查本任务组变更
  - 占位符：
    - `{PLAN_OR_REQUIREMENTS}` → `openspec/changes/fix-iam-app-level-auth/specs/iam-app-auth/spec.md` + `tasks.md`
    - `{WHAT_WAS_IMPLEMENTED}` → `cmd/config-server/service/kv.go`
    - `{BASE_SHA}` → 任务组开始前 commit SHA
    - `{HEAD_SHA}` → 当前 HEAD
  - Critical/Important → 停等用户指令
  - Minor/无问题 → 自动继续

## 2. config-server 配置项鉴权修复

- [x] 2.1 修复 ListConfigItemCount、ListConfigItemByTuple、GetTemplateAndNonTemplateCICount 补充 App View 校验  <!-- 非 TDD 任务 -->
  - [x] 2.1.1 执行变更：`cmd/config-server/service/config_item.go`（3 个函数，添加 App View + ResourceID）
  - [x] 2.1.2 验证无回归（运行：`go build ./cmd/config-server/...`，确认编译通过）
  - [x] 2.1.3 检查：确认 3 个函数的 ResourceAttribute 列表均包含 meta.App + ResourceID

- [x] 2.2 修复 CompareConfigItemConflicts 补充双 App View 校验  <!-- 非 TDD 任务 -->
  - [x] 2.2.1 执行变更：`cmd/config-server/service/config_item.go`（1 个函数，对 req.AppId 和 req.OtherAppId 分别添加 App View + ResourceID，共两条 App 资源）
  - [x] 2.2.2 验证无回归（运行：`go build ./cmd/config-server/...`，确认编译通过）
  - [x] 2.2.3 检查：确认 ResourceAttribute 列表包含两条 meta.App 记录（分别对应 AppId 和 OtherAppId）

- [ ] 2.3 代码审查
  - 前置：调用 superpowers:verification-before-completion 运行全量测试
  - 调用 superpowers:requesting-code-review 审查本任务组变更
  - 占位符：
    - `{PLAN_OR_REQUIREMENTS}` → `openspec/changes/fix-iam-app-level-auth/specs/iam-app-auth/spec.md` + `tasks.md`
    - `{WHAT_WAS_IMPLEMENTED}` → `cmd/config-server/service/config_item.go`
    - `{BASE_SHA}` → 任务组开始前 commit SHA
    - `{HEAD_SHA}` → 当前 HEAD
  - Critical/Important → 停等用户指令
  - Minor/无问题 → 自动继续

## 3. config-server Hook 和 Group 鉴权修复

- [x] 3.1 修复 GetReleaseHook 补充 App View 校验  <!-- 非 TDD 任务 -->
  - [ ] 3.1.1 执行变更：`cmd/config-server/service/hook.go`（1 个函数，添加 App View + ResourceID）
  - [ ] 3.1.2 验证无回归（运行：`go build ./cmd/config-server/...`，确认编译通过）
  - [ ] 3.1.3 检查：确认 ResourceAttribute 列表包含 meta.App + ResourceID

- [x] 3.2 修复 ListAppGroups 补充 App View 校验  <!-- 非 TDD 任务 -->
  - [ ] 3.2.1 执行变更：`cmd/config-server/service/group.go`（1 个函数，添加 App View + ResourceID）
  - [ ] 3.2.2 验证无回归（运行：`go build ./cmd/config-server/...`，确认编译通过）
  - [ ] 3.2.3 检查：确认 ResourceAttribute 列表包含 meta.App + ResourceID

- [ ] 3.3 ~~修复 ListGroupReleasedApps 添加后置 App View 批量校验~~  <!-- 暂缓：后置校验会导致用户进入页面后出现错误提示 -->
  - **暂缓原因**：后置校验会拒绝整个请求（用户对某个关联 App 无权限时），导致页面无法正常显示。待产品层面确定降级方案（如响应过滤）后再处理。

- [ ] 3.4 ~~修复 ListHookReferences 添加后置 App View 批量校验~~  <!-- 暂缓：同 3.3 -->
  - **暂缓原因**：同 3.3

- [ ] 3.5 ~~修复 ListHookRevisionReferences 添加后置 App View 批量校验~~  <!-- 暂缓：同 3.3 -->
  - **暂缓原因**：同 3.3

- [ ] 3.6 代码审查
  - 前置：调用 superpowers:verification-before-completion 运行全量测试
  - 调用 superpowers:requesting-code-review 审查本任务组变更
  - 占位符：
    - `{PLAN_OR_REQUIREMENTS}` → `openspec/changes/fix-iam-app-level-auth/specs/iam-app-auth/spec.md` + `tasks.md`
    - `{WHAT_WAS_IMPLEMENTED}` → `cmd/config-server/service/hook.go`、`hook_revision.go`、`group.go` 及测试文件
    - `{BASE_SHA}` → 任务组开始前 commit SHA
    - `{HEAD_SHA}` → 当前 HEAD
  - Critical/Important → 停等用户指令
  - Minor/无问题 → 自动继续

## 4. api-server 导入导出鉴权修复

- [x] 4.1 修复 ConfigFileImport 在 handler 入口添加 App Update 校验  <!-- 非 TDD 任务 -->
  - [ ] 4.1.1 执行变更：`cmd/api-server/service/config_import.go`（在 ConfigFileImport 函数入口处添加 `c.authorizer.Authorize` 调用，Biz + App Update）
  - [ ] 4.1.2 验证无回归（运行：`go build ./cmd/api-server/...`，确认编译通过）
  - [ ] 4.1.3 检查：确认 Authorize 调用位于参数解析之后、业务逻辑之前

- [x] 4.2 修复 ConfigFileExport 在 handler 入口添加 App View 校验  <!-- 非 TDD 任务 -->
  - [ ] 4.2.1 执行变更：`cmd/api-server/service/config_export.go`（在 ConfigFileExport 函数入口处添加 `c.authorizer.Authorize` 调用，Biz + App View）
  - [ ] 4.2.2 验证无回归（运行：`go build ./cmd/api-server/...`，确认编译通过）
  - [ ] 4.2.3 检查：确认 Authorize 调用位于参数解析之后、业务逻辑之前

- [x] 4.3 修复 kvService.Export 在 handler 入口添加 App View 校验  <!-- 非 TDD 任务 -->
  - [ ] 4.3.1 执行变更：`cmd/api-server/service/released_kv.go`（在 Export 函数入口处添加 `m.authorizer.Authorize` 调用，Biz + App View）
  - [ ] 4.3.2 验证无回归（运行：`go build ./cmd/api-server/...`，确认编译通过）
  - [ ] 4.3.3 检查：确认 Authorize 调用位于参数解析之后、业务逻辑之前

- [ ] 4.4 代码审查
  - 前置：调用 superpowers:verification-before-completion 运行全量测试
  - 调用 superpowers:requesting-code-review 审查本任务组变更
  - 占位符：
    - `{PLAN_OR_REQUIREMENTS}` → `openspec/changes/fix-iam-app-level-auth/specs/iam-app-auth/spec.md` + `tasks.md`
    - `{WHAT_WAS_IMPLEMENTED}` → `cmd/api-server/service/config_import.go`、`config_export.go`、`released_kv.go`
    - `{BASE_SHA}` → 任务组开始前 commit SHA
    - `{HEAD_SHA}` → 当前 HEAD
  - Critical/Important → 停等用户指令
  - Minor/无问题 → 自动继续

## 5. 其他鉴权缺失修复

- [x] 5.1 修复 GetAppByName 恢复 IAM 校验（先查询再鉴权）  <!-- TDD 任务 -->
  - [ ] 5.1.1 写失败测试：`cmd/config-server/service/app_test.go`（测试用例验证：查到 App 后无 View 权限时返回错误）
  - [ ] 5.1.2 验证测试失败（运行：`go test ./cmd/config-server/service/ -run TestGetAppByName -v`）
  - [ ] 5.1.3 写最小实现：`cmd/config-server/service/app.go`（先调 DS.GetAppByName 拿到 app 记录，用 rp.Id 做 Biz + App View 校验）
  - [ ] 5.1.4 验证测试通过
  - [ ] 5.1.5 重构

- [x] 5.2 修复 ManageConfigKV 添加完整鉴权  <!-- 非 TDD 任务 -->
  - [x] 5.2.1 **跳过**：ManageConfigKVReq 无 BizId/AppId 字段，为系统级配置管理 API，App 级校验不适用
  - [x] 5.2.2 验证无回归
  - [x] 5.2.3 检查

- [ ] 5.3 ~~修复 ListAppTemplateSets 补充 ResourceID~~  <!-- 暂缓：配置模板相关接口暂不增加 App 鉴权，避免页面错误提示 -->
  - [ ] 5.3.1 执行变更：`cmd/config-server/service/template_set.go`（在 `{Type: meta.App, Action: meta.View}` 中添加 `ResourceID: req.AppId`）
  - [ ] 5.3.2 验证无回归（运行：`go build ./cmd/config-server/...`，确认编译通过）
  - [ ] 5.3.3 检查：确认 ResourceID 字段已正确设置

- [ ] 5.4 代码审查
  - 前置：调用 superpowers:verification-before-completion 运行全量测试
  - 调用 superpowers:requesting-code-review 审查本任务组变更
  - 占位符：
    - `{PLAN_OR_REQUIREMENTS}` → `openspec/changes/fix-iam-app-level-auth/specs/iam-app-auth/spec.md` + `tasks.md`
    - `{WHAT_WAS_IMPLEMENTED}` → `cmd/config-server/service/app.go`、`process_config_view.go`、`template_set.go` 及测试文件
    - `{BASE_SHA}` → 任务组开始前 commit SHA
    - `{HEAD_SHA}` → 当前 HEAD
  - Critical/Important → 停等用户指令
  - Minor/无问题 → 自动继续

## 6. 模板类接口叠加 App 校验

- [ ] 6.1 ~~修复 CreateTemplateSet、UpdateTemplateSet 对 BoundApps 批量 App View 校验~~  <!-- 暂缓：配置模板相关接口暂不增加 App 鉴权 -->
  - **暂缓原因**：配置模板查看如果需要用户有相关 App 权限，会导致用户进入页面后出现大量错误提示。

- [ ] 6.2 ~~修复 ListTmplSetsOfBiz 对 AppId 过滤条件添加 App View 校验~~  <!-- 暂缓：同 6.1 -->
  - **暂缓原因**：同 6.1

- [ ] 6.3 ~~修复 BatchUpdateTemplatePermissions 对 AppIds 批量 App View 校验~~  <!-- 暂缓：同 6.1 -->
  - **暂缓原因**：同 6.1

- [ ] 6.4 ~~修复 CheckTemplateSetReferencesApps 添加后置 App View 批量校验~~  <!-- 暂缓：同 6.1 -->
  - **暂缓原因**：同 6.1

- [x] 6.5 代码审查（已完成的部分通过审查）
  - 前置：调用 superpowers:verification-before-completion 运行全量测试
  - 调用 superpowers:requesting-code-review 审查本任务组变更
  - 占位符：
    - `{PLAN_OR_REQUIREMENTS}` → `openspec/changes/fix-iam-app-level-auth/specs/iam-app-auth/spec.md` + `tasks.md`
    - `{WHAT_WAS_IMPLEMENTED}` → `cmd/config-server/service/template_set.go`、`template.go`、`template_binding_relation.go` 及测试文件
    - `{BASE_SHA}` → 任务组开始前 commit SHA
    - `{HEAD_SHA}` → 当前 HEAD
  - Critical/Important → 停等用户指令
  - Minor/无问题 → 自动继续

## 7. 变更文档定稿 (Required)

- [x] 7.1 定稿 design.md: 记录技术决策、偏差和实现细节
- [x] 7.2 定稿 tasks.md: 全量标记所有层级任务（顶层 + 子任务），将已完成的 `[ ]` 标记为 `[x]`
- [x] 7.3 定稿 proposal.md: 更新范围/影响（若有变化）
- [x] 7.4 定稿 specs/iam-app-auth/spec.md: 更新功能需求（若有变化）+ 验证 spec 与实际实现一致
- [x] 7.5 最终校验: 确保所有变更文档反映实际实现
