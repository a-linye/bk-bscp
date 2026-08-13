# 技术规范

本目录中的技术规范文件由 Harness 预设同步，预设文件是唯一权威来源，不得在本目录直接定制。需要调整规范时，应修改 Harness 预设后重新同步。预设中的目录与版本是通用基线；bk-bscp 的当前事实以 `go.mod`、`ui/package.json` 和 [`../harness/architectural-constraints.md`](../harness/architectural-constraints.md) 为准，不在无关需求中强制升级历史技术栈。

## 必选规范

| 规范 | 文件 | 级别 |
|------|------|------|
| 蓝鲸代码安全红线 | [security-bk-redlines.md](security-bk-redlines.md) | Level 1 |
| 代码评审 | [quality-code-review.md](quality-code-review.md) | Level 1 |

## 当前项目选用的规范

| 范畴 | 技术栈 | 文件 | 级别 |
|------|--------|------|------|
| 前端 | Vue 3 | [frontend-vue3.md](frontend-vue3.md) | Level 1 |
| 接口 | gRPC-Gateway | [api-grpc-gateway.md](api-grpc-gateway.md) | Level 1 |
| 后端 | Go + gRPC | [backend-generic.md](backend-generic.md) | Level 2 |

## 项目适用性说明

- `frontend-vue3.md` 的 Vue 3.5+/TypeScript 5.x 是预设目标基线；当前仓库为 Vue 3.4.21/TypeScript 4.6.4，未安排升级时不得引入仅新版本支持的语法。
- `api-grpc-gateway.md` 的架构示例包含 go-micro，但 bk-bscp 源码没有 go-micro import；本项目只采用其中 Protobuf、gRPC-Gateway 和接口兼容性约束。
- `backend-generic.md` 是通用骨架，项目目录和分层以 Harness 架构约束为准。

## Agent 加载策略

- 任何代码任务都加载安全红线和代码评审规范。
- 前端任务按需加载前端 Vue 3 规范。
- 接口任务按需加载接口 gRPC-Gateway 规范。
- 后端任务按需加载 Go + gRPC 通用规范和项目架构约束。
- 全栈任务加载以上全部规范。

## 章节索引

### security-bk-redlines.md

- 一级标题：代码安全规范（蓝鲸三大红线）
- 二级标题：
  - 一、红线总览
  - 二、红线 1：外部输入未校验
  - 三、红线 2：敏感接口未鉴权
  - 四、红线 3：敏感数据未加密
  - 五、代码评审检查清单

### quality-code-review.md

- 一级标题：无
- 二级标题：
  - 一、核心原则
  - 二、问题分级
  - 三、检查维度与规则清单
  - 四、代码质量评分标准
  - 五、评审意见撰写规范
  - 六、评审报告格式

### frontend-vue3.md

- 一级标题：前端开发规范
- 二级标题：
  - 一、技术栈要求
  - 二、项目结构
  - 三、编码规范
  - 四、状态管理规范（Pinia）
  - 五、网络请求规范
  - 六、三层代码架构
  - 七、Vue 3 最佳实践
  - 八、UI 组件使用规范
  - 九、质量保证
  - 十、常见陷阱与避坑

### api-grpc-gateway.md

- 一级标题：前后端接口规范
- 二级标题：
  - 一、架构概述
  - 二、接口定义规范（Proto）
  - 三、数据类型映射
  - 四、响应格式规范
  - 五、前端接入规范
  - 六、核心契约模式
  - 七、联调标准流程
  - 八、API 网关配置规范
  - 九、常见陷阱与解决方案
  - 十、附录

### backend-generic.md

- 一级标题：后端开发规范
- 二级标题：
  - 一、技术栈要求
  - 二、项目结构
  - 三、接口定义规范
  - 四、分层架构
  - 五、编码规范
  - 六、错误处理
  - 七、配置管理
  - 八、日志规范
  - 九、测试规范
  - 十、安全规范
  - 十一、构建与部署
  - 十二、服务注册与通信

## 完善状态

| 分类 | 当前状态 | 技术栈 | 如何完善 |
|------|---------|--------|---------|
| 后端 | 通用骨架（待补充） | Go + gRPC | 编写完整规范，放入 Harness 预设库并注册 `index.yaml` |

前端、接口、安全和质量规范为 Level 1；后端因预设库没有匹配当前源码的 Go + gRPC 预设，降级为 Level 2。项目实际目录、版本和依赖规则以 `go.mod` 与 Harness 架构约束为准。
