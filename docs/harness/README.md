# Harness Engineering 规范

> 本目录定义 bk-bscp 的 AI Agent 运行环境规范。入口只提供导航，执行任务时按需加载对应组件，避免无关上下文占用。

## 项目概述

- **项目名称**：蓝鲸服务配置中心（bk-bscp）
- **技术栈**：Go 1.25、gRPC/gRPC-Gateway、MySQL、Redis、Vue 3、TypeScript、Vite
- **Agent 适用场景**：需求澄清与评估、方案设计、功能开发、缺陷修复、代码评审、安全检查、配置只读查询及 Harness 文档维护

## 规范导航

| 组件 | 文档 | 概要 |
|------|------|------|
| 上下文工程 | [context-engineering.md](context-engineering.md) | 知识来源、渐进式披露和动态数据接入 |
| 架构约束 | [architectural-constraints.md](architectural-constraints.md) | 服务分层、依赖方向、边界和数据入口 |
| 熵管理 | [entropy-management.md](entropy-management.md) | 文档园艺、生成物一致性和技术债追踪 |
| 工具能力 | [tooling.md](tooling.md) | Skill、MCP、CLI、接口及安全约束 |
| 执行与验证 | [execution-verification.md](execution-verification.md) | 任务循环、变更验证、漂移检测和记录 |

## 关联入口

- [技术开发规范](../standards/README.md)：实现和评审时按变更端加载
- [开发地图](../dev-map/README.md)：查询代码结构、概念关联与依赖路径
- [项目词汇表](../glossary.md)：业务与工程术语的统一解释
- [迭代开发工作流](../workflow.md)：TAPD 驱动的需求到交付步骤

## 加载策略

1. 首次接触仓库先读根目录 `AGENTS.md` 和本文件。
2. 代码定位优先查询开发地图；图谱没有证据时再搜索源码。
3. 修改代码前加载架构约束、涉及技术栈规范以及对应测试。
4. 涉及外部系统、提交、生产配置或删除操作时，同时加载工具能力与安全规范。
5. 宣称完成前按执行与验证清单取得新的验证证据。

## 维护规则

- 架构、技术栈、构建命令、Skill 或工作流变化时，同一变更内同步更新相关文档。
- 运行 `harness-engineering` 的文档园艺模式检查代码与规范的一致性。
- `docs/standards/` 由 Harness 预设同步，不直接手工定制；项目特有约束写入本目录。

## 版本记录

| 版本 | 日期 | 变更说明 |
|------|------|---------|
| 1.0.0 | 2026-07-29 | 初始生成 |
