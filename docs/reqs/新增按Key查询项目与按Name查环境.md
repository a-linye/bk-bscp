# 新增按 Key 查询项目与按 Name 查询环境接口

## 基本信息

| 字段 | 值 |
|------|-----|
| 父需求短 ID | 136928701 |
| 父需求长 ID | 1120451610136928701 |
| 子需求短 ID | 136928904 |
| 子需求长 ID | 1120451610136928904 |
| 优先级 | Middle |
| 依赖 | 无（与另一子需求可并行） |
| 预估工时 | 16 人时（2 人天） |
| 价值规模 | 21.25（Reach=80, Impact=5, Confidence=85%, Effort=2人天） |

## 用户故事

- 作为 **配置中心调用方（Sidecar / 前端 / 外部集成系统）**，我想要通过 ProjectKey 直接查询项目、通过 EnvName（结合项目）直接查询环境，以便于无需先持有内部数值 ID 即可定位资源。

## 核心功能点

- **F-001 按 ProjectKey 查询项目**：输入 BizId、ProjectKey；config-server 校验 BizId 查询权限（FindBusinessResource）后调用 data-service `GetProjectByKey`，按 (BizID, Key) 查询，未命中返回 RecordNotFound（project does not exist）。
- **F-002 按 EnvName 查询环境**：输入 BizId、ProjectId、EnvName；config-server 校验权限后调用 data-service `GetEnvironmentByName`，按 (BizID, ProjectID, EnvName) 查询，未命中返回 RecordNotFound。

## 验收标准

- **AC-001**：合法 BizId + 存在的 ProjectKey → 返回项目 Id/Spec/Attachment
- **AC-002**：不存在的 ProjectKey → RecordNotFound
- **AC-003**：合法 BizId/ProjectId + 存在的 EnvName → 返回环境 Id/Spec/Attachment
- **AC-004**：不存在的 EnvName → RecordNotFound
- **AC-005**：无 BizId 查询权限 → 鉴权失败

## 边界范围

- 包含：两个查询接口（config-server 接入鉴权 + data-service DAO/proto 实现 + 协议定义）
- 不包含：Sidecar 元数据字段改造（见另一子需求）

## 人力与工时

- 全量工作 1 位高级工程师完成工时预估：16 人时（2 人天，含开发 + 单测 + 代码审查 + 文档）
- 全量工作 1 位中级工程师完成工时预估：约 22 人时（约 2.75 人天）

## 评分依据

- Reach=80：影响所有依赖项目/环境定位的调用方（Sidecar、前端、外部集成），属高频基础能力
- Impact=5（中优先级）：重要功能改进，降低 ID 转换耦合
- Confidence=85%（中高信心）：接口契约已随代码落地，方案明确
- Effort=2 人天
- RICE = (80 × 5 × 0.85) / 2 = 170
