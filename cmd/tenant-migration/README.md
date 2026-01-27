BSCP 租户迁移工具
==================

将 BSCP 单租户环境数据迁移到多租户环境。

### 前置准备
1. 停止源环境的写入操作（避免迁移过程中数据变更）
2. 备份目标数据库（确保可以回滚）
3. 确认迁移机器可以访问源/目标的 MySQL 和 Vault
4. 确认 MySQL 用户有读写权限，Vault Token 有 KV 读写权限
5. 准备配置文件 `migration.yaml`，参考 `etc/migration.yaml` 模板

### 迁移方式一：全量迁移（MySQL + Vault）
按顺序迁移 MySQL 数据（30张核心表）和 Vault KV 数据：
- 使用方式
  ```
  ./bk-bscp-tenant-migration migrate all [flags]
  ```
- 命令行参数
  ```
  -c, --config string         配置文件路径（必填）
      --biz-ids uint32Slice   要迁移的业务ID列表（逗号分隔，覆盖配置文件）
  ```
- 示例
  ```
  # 迁移所有业务
  ./bk-bscp-tenant-migration -c migration.yaml migrate all
  
  # 只迁移指定业务
  ./bk-bscp-tenant-migration -c migration.yaml migrate all --biz-ids=100,200,300
  ```

### 迁移方式二：仅MySQL迁移
仅迁移 MySQL 数据，适用于不使用 Vault 存储 KV 值的场景：
- 使用方式
  ```
  ./bk-bscp-tenant-migration migrate mysql [flags]
  ```
- 命令行参数
  ```
  -c, --config string         配置文件路径（必填）
      --biz-ids uint32Slice   要迁移的业务ID列表（逗号分隔，覆盖配置文件）
  ```
- 示例
  ```
  ./bk-bscp-tenant-migration -c migration.yaml migrate mysql --biz-ids=100,200
  ```

### 迁移方式三：仅Vault迁移
仅迁移 Vault KV 数据，需确保 MySQL 数据已迁移（依赖 `kvs` 和 `released_kvs` 表）：
- 使用方式
  ```
  ./bk-bscp-tenant-migration migrate vault [flags]
  ```
- 命令行参数
  ```
  -c, --config string         配置文件路径（必填）
      --biz-ids uint32Slice   要迁移的业务ID列表（逗号分隔，覆盖配置文件）
  ```
- 示例
  ```
  ./bk-bscp-tenant-migration -c migration.yaml migrate vault --biz-ids=100,200
  ```

### 数据验证
验证源表与目标表记录数是否一致，以及 tenant_id 是否正确填充：
- 使用方式
  ```
  ./bk-bscp-tenant-migration validate [flags]
  ```
- 命令行参数
  ```
  -c, --config string         配置文件路径（必填）
      --biz-ids uint32Slice   要验证的业务ID列表（逗号分隔，覆盖配置文件）
  ```
- 示例
  ```
  ./bk-bscp-tenant-migration -c migration.yaml validate --biz-ids=100,200,300
  ```

### 数据清理
清理目标数据库中的迁移数据，用于迁移失败后重新执行：
- 使用方式
  ```
  ./bk-bscp-tenant-migration cleanup [flags]
  ```
- 命令行参数
  ```
  -c, --config string         配置文件路径（必填）
  -f, --force                 跳过确认提示
      --biz-ids uint32Slice   要清理的业务ID列表（逗号分隔，覆盖配置文件）
  ```
- 示例
  ```
  # 清理所有数据（交互式确认）
  ./bk-bscp-tenant-migration -c migration.yaml cleanup
  
  # 只清理指定业务的数据
  ./bk-bscp-tenant-migration -c migration.yaml cleanup --biz-ids=100,200 -f
  ```

### 按业务维度迁移
支持只迁移指定业务（biz_id）的数据，有两种方式指定业务ID：

**方式一：通过命令行参数（推荐）**
```bash
./bk-bscp-tenant-migration -c migration.yaml migrate all --biz-ids=100,200,300
```

**方式二：通过配置文件**
```yaml
migration:
  target_tenant_id: "your_tenant_id"
  biz_ids:
    - 100
    - 200
    - 300
```

> 注意：命令行参数 `--biz-ids` 会覆盖配置文件中的 `biz_ids` 设置。

适用场景：分批迁移大量业务数据、只迁移部分测试业务进行验证、针对特定业务进行数据回滚和重新迁移。

### 配置文件说明
配置文件使用 YAML 格式，主要配置项：

| 配置项 | 必填 | 说明 |
|--------|------|------|
| `migration.target_tenant_id` | 是 | 目标租户 ID，将填充到所有迁移记录 |
| `migration.biz_ids` | 否 | 要迁移的业务 ID 列表，不填则迁移所有业务 |
| `migration.batch_size` | 否 | 批量处理大小，默认 1000 |
| `migration.dry_run` | 否 | 试运行模式，true 时只读取不写入 |
| `migration.continue_on_error` | 否 | 遇错继续，true 时跳过失败记录 |
| `source.mysql.*` | 是 | 源 MySQL 连接配置 |
| `source.vault.*` | 否 | 源 Vault 配置（迁移 Vault 时必填） |
| `target.mysql.*` | 是 | 目标 MySQL 连接配置 |
| `target.vault.*` | 否 | 目标 Vault 配置（迁移 Vault 时必填） |

完整配置示例见 `etc/migration.yaml`。

### 迁移流程
```
1. 准备配置文件
   ↓
2. 试运行测试（设置 dry_run: true）
   ./bk-bscp-tenant-migration -c migration.yaml migrate all
   ↓
3. 正式迁移（设置 dry_run: false）
   ./bk-bscp-tenant-migration -c migration.yaml migrate all
   ↓
4. 验证数据
   ./bk-bscp-tenant-migration -c migration.yaml validate
   ↓
5. 验证失败？清理后重试
   ./bk-bscp-tenant-migration -c migration.yaml cleanup -f
   ./bk-bscp-tenant-migration -c migration.yaml migrate all
```

### 迁移的表
**核心业务表（30张）**：sharding_bizs、applications、template_spaces、groups、hooks、credentials、config_items、releases、strategy_sets、template_sets、templates、template_variables、hook_revisions、credential_scopes、group_app_binds、commits、contents、strategies、current_published_strategies、kvs、released_config_items、released_groups、released_hooks、released_kvs、template_revisions、app_template_bindings、app_template_variables、released_app_templates、released_app_template_variables、id_generators

**跳过的表（运行时/历史数据）**：events、current_released_instances、resource_locks、clients、client_events、client_querys、audits、published_strategy_histories、archived_apps、configs、schema_migrations、biz_hosts、sharding_dbs
