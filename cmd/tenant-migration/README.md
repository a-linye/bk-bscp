BSCP 租户迁移工具
==================

将 BSCP 单租户环境数据迁移到多租户环境。

## 功能概览

| 命令 | 功能 |
|------|------|
| `migrate` | 执行数据迁移（MySQL + Vault） |
| `cleanup` | 清理目标数据库中的迁移数据 |
| `scan` | 扫描源/目标数据库资产 |

## 前置准备

1. 停止源环境的写入操作（避免迁移过程中数据变更）
2. 备份目标数据库（确保可以回滚）
3. 确认迁移机器可以访问源/目标的 MySQL 和 Vault
4. 确认 MySQL 用户有读写权限，Vault Token 有 KV 读写权限
5. 准备配置文件 `migration.yaml`，参考 `etc/migration.yaml` 模板

## 命令说明

### migrate - 数据迁移

按依赖顺序迁移 MySQL 数据（30张核心表）和 Vault KV 数据。

```bash
# 迁移所有业务
./bk-bscp-tenant-migration -c migration.yaml migrate

# 只迁移指定业务（推荐用于分批迁移或测试）
./bk-bscp-tenant-migration -c migration.yaml migrate --biz-ids=100,200,300
```

**参数说明**：

| 参数 | 必填 | 说明 |
|------|------|------|
| `-c, --config` | 是 | 配置文件路径 |
| `--biz-ids` | 否 | 要迁移的业务ID列表（逗号分隔），覆盖配置文件设置 |

> 如果配置文件中未配置 Vault，则只迁移 MySQL 数据。

### cleanup - 数据清理

清理目标数据库中的迁移数据，用于迁移失败后重新执行。

```bash
# 清理指定业务数据（交互式确认）
./bk-bscp-tenant-migration -c migration.yaml cleanup --biz-ids=100,200

# 跳过确认直接清理
./bk-bscp-tenant-migration -c migration.yaml cleanup --biz-ids=100,200 -f
```

**参数说明**：

| 参数 | 必填 | 说明 |
|------|------|------|
| `-c, --config` | 是 | 配置文件路径 |
| `-f, --force` | 否 | 跳过确认提示 |
| `--biz-ids` | 否 | 要清理的业务ID列表（逗号分隔），覆盖配置文件设置 |

### scan - 资产扫描

扫描源/目标数据库的表和记录数，用于迁移前后对比验证。

```bash
# 扫描配置的迁移表
./bk-bscp-tenant-migration -c migration.yaml scan

# 扫描数据库中所有表
./bk-bscp-tenant-migration -c migration.yaml scan --all
```

## 配置文件说明

配置文件使用 YAML 格式，完整示例见 `etc/migration.yaml`。

| 配置项 | 必填 | 说明 |
|--------|------|------|
| `migration.target_tenant_id` | 是 | 目标租户 ID，将填充到所有迁移记录 |
| `migration.biz_ids` | 否 | 要迁移的业务 ID 列表，不填则迁移所有业务 |
| `migration.batch_size` | 否 | 批量处理大小，默认 1000 |
| `migration.continue_on_error` | 否 | 遇错继续，true 时跳过失败记录 |
| `source.mysql.*` | 是 | 源 MySQL 连接配置 |
| `source.vault.*` | 否 | 源 Vault 配置（迁移 Vault 时必填） |
| `target.mysql.*` | 是 | 目标 MySQL 连接配置 |
| `target.vault.*` | 否 | 目标 Vault 配置（迁移 Vault 时必填） |

## 典型使用场景

### 场景一：全量迁移

迁移所有业务数据到新环境：

```bash
./bk-bscp-tenant-migration -c migration.yaml migrate
```

### 场景二：分批迁移

当业务数据量较大时，建议分批迁移：

```bash
# 第一批
./bk-bscp-tenant-migration -c migration.yaml migrate --biz-ids=100,101,102

# 第二批
./bk-bscp-tenant-migration -c migration.yaml migrate --biz-ids=200,201,202
```

### 场景三：测试验证

先迁移少量业务进行验证：

```bash
# 迁移测试业务
./bk-bscp-tenant-migration -c migration.yaml migrate --biz-ids=100

# 验证无误后，继续迁移其他业务
./bk-bscp-tenant-migration -c migration.yaml migrate --biz-ids=200,300
```

### 场景四：迁移失败重试

如果迁移过程中出现问题，需要重新迁移：

```bash
# 清理已迁移的数据
./bk-bscp-tenant-migration -c migration.yaml cleanup --biz-ids=100 -f

# 重新迁移
./bk-bscp-tenant-migration -c migration.yaml migrate --biz-ids=100
```

## 业务侧配合事项

迁移完成后，需要通知业务侧进行以下配置调整：

### 1. 更新客户端 Feed-Server 地址

客户端需要更新 Feed-Server 连接地址为：`bscp-feed.sg.bk2game.com`

详细配置方法请参考：[BSCP 客户端配置指南](https://iwiki.woa.com/p/4008897780)

### 2. 验证配置拉取

1. 确认客户端能够正常连接新的 Feed-Server
2. 验证配置拉取功能正常
3. 检查业务配置数据完整性
