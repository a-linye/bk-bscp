# AGENTS.md

> Agent 认知 bk-bscp 的第一站：先确认任务边界，再按需加载详细规范与开发地图。

## 项目概述

- **项目名称**：蓝鲸服务配置中心（BlueKing Service Configuration Provider，BSCP）
- **仓库地址**：`github.com/TencentBlueKing/bk-bscp`
- **定位**：面向业务的服务配置管理平台，支持文件、KV、表格配置以及版本、灰度、模板化管理
- **主要技术栈**：Go 1.25、gRPC/gRPC-Gateway、MySQL、Redis、Vue 3、TypeScript、Vite

## 目录结构

```text
cmd/                 服务入口、装配、应用服务与进程构建
internal/            仓库内部实现：DAL、外部组件适配、处理器、任务与运行时
pkg/                 跨模块公共协议、类型、校验、数据表模型与基础能力
ui/                  Vue 3 管理端
docs/                产品、需求、接口与工程文档
├── harness/         AI Agent 运行环境规范
├── standards/       安全、质量和技术栈开发规范
├── dev-map/         graphify 知识图谱及使用说明
└── workflow.md      TAPD 驱动的迭代开发工作流
scripts/             构建、安装、代码生成和发布脚本
test/                集成、基准与 mock 测试
```

## 关键规范

- Harness 规范 → [`docs/harness/README.md`](docs/harness/README.md)
- 技术开发规范 → [`docs/standards/README.md`](docs/standards/README.md)
- 项目术语 → [`docs/glossary.md`](docs/glossary.md)
- 开发地图 → [`docs/dev-map/README.md`](docs/dev-map/README.md)
- 贡献与设计文档要求 → [`CONTRIBUTING.md`](CONTRIBUTING.md)

## 协作约束

- 方案、计划、评审、提交说明和非显然业务注释优先使用中文；协议字段、配置项和既有英文术语保持原名。
- 先读取任务涉及目录的现有实现和测试，保持改动小、边界清晰，不引入需求外抽象或兼容层。
- 不回滚用户已有改动；提交前检查 `git status --short`，避免混入无关文件。
- Go 改动须符合 `.golangci.yml`，运行 `gofmt`，并优先执行受影响包测试；行为变更应补测试。
- 协议、生成代码或 Swagger 变更使用仓库既有生成命令，并确认生成后 `git diff`。
- 禁止绕过 GPG 签名、提交钩子或安全检查；签名缓存失效时由用户在终端暖缓存。

## 开发工作流

本项目使用 `workflow-agent` 按 [`docs/workflow.md`](docs/workflow.md) 定义的步骤推进迭代开发。workflow-agent 启动时主动感知当前状态（首次执行、崩溃恢复、错误暂停、重新开始），无需用户输入特定指令。不允许跳过工作流步骤或自行决定开发流程。
