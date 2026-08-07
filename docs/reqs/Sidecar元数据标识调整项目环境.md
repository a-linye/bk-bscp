# Sidecar 元数据标识调整为 ProjectKey 与 EnvName

## 基本信息

| 字段 | 值 |
|------|-----|
| 父需求短 ID | 136928701 |
| 父需求长 ID | 1120451610136928701 |
| 子需求短 ID | 136928907 |
| 子需求长 ID | 1120451610136928907 |
| 优先级 | Middle |
| 依赖 | 无（与另一子需求可并行，同属功能面） |
| 预估工时 | 12 人时（1.5 人天） |
| 价值规模 | 28.33（Reach=100, Impact=5, Confidence=50%, Effort=1.5人天） |

## 用户故事

- 作为 **Sidecar 上报链路**，我在元数据中使用 ProjectKey / EnvName 而非数值 ID，以便于与业务侧标识体系保持一致，降低标识转换成本。

## 核心功能点

- **F-003 Sidecar 元数据标识调整**：`SidecarMetaHeader` 结构体字段由 `ProjectID uint32` / `EnvID uint32` 调整为 `ProjectKey string` / `EnvName string`；下游 cache-service / feed-server 等按新字符串标识识别项目与环境。

## 验收标准

- **AC-006**：Sidecar 上报元数据，服务端解析 SidecarMetaHeader 以 ProjectKey/EnvName 完成项目/环境识别

## 破坏性变更说明

- ⚠️ 字段类型由 uint32 改为 string，旧版 Sidecar 上报的 ProjectID/EnvID 字段不再适用。需明确升级策略：强制升级 或 双字段过渡兼容（Q-001，待确认）。
- 兼容方案未定前，Confidence 取保守值。

## 边界范围

- 包含：SidecarMetaHeader 字段改造及下游消费方（cache-service / feed-server）适配
- 不包含：查询接口本身（见另一子需求）

## 人力与工时

- 全量工作 1 位高级工程师完成工时预估：12 人时（1.5 人天，含开发 + 单测 + 代码审查 + 文档）
- 全量工作 1 位中级工程师完成工时预估：约 16 人时（约 2 人天）

## 评分依据

- Reach=100：Sidecar 上报链路为全量流量入口，所有项目/环境识别均受影响
- Impact=5（中优先级）：但属破坏性变更，落地后与查询接口协同才能完整闭环
- Confidence=50%（低信心）：兼容方案（Q-001）未确认，存在落地不确定性
- Effort=1.5 人天
- RICE = (100 × 5 × 0.5) / 1.5 ≈ 166.67
