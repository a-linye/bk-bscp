# BSCP 租户迁移工具使用指南

## 一、工具简介

本工具用于将 BSCP 单租户环境（v2.3.13）的数据迁移到多租户环境（v2.3.8-multi-tenant）。

**迁移范围**：

| 数据类型 | 迁移方式 | 说明 |
|----------|----------|------|
| MySQL | 直接迁移 + 填充 tenant_id | 29 张核心业务表 |
| Vault KV | 通过 API 读写 | 因两套环境密钥不同，必须通过 API 迁移 |
| ITSM | 不迁移 | 新环境重新配置 |
| BkRepo | 不迁移 | 建议共用同一 BkRepo 实例 |

---

## 二、使用前准备

### 2.1 环境信息收集

在开始迁移前，请准备以下信息：

| 项目 | 源环境 | 目标环境 |
|------|--------|----------|
| MySQL 地址 | | |
| MySQL 数据库名 | | |
| MySQL 用户名/密码 | | |
| Vault 地址 | | |
| Vault Token | | |
| 目标租户 ID | - | |

### 2.2 前置检查

1. **停止源环境写入**：暂停源环境的写入操作
2. **备份目标数据库**：确保可以回滚
3. **网络连通性**：确认迁移工具所在机器可以访问源/目标的 MySQL 和 Vault
4. **权限检查**：确认 MySQL 用户有读写权限，Vault Token 有 KV 读写权限

---

## 三、配置文件说明

配置文件使用 YAML 格式，参考模板 `etc/migration.yaml`：

```yaml
# 迁移设置
migration:
  # 目标租户ID（必填）- 将填充到所有迁移记录的 tenant_id 字段
  target_tenant_id: "your_tenant_id"
  
  # 每批处理的记录数，默认 1000
  batch_size: 1000
  
  # 试运行模式：true 时只读取数据不写入，用于测试
  dry_run: false
  
  # 遇到错误是否继续：true 时跳过失败记录继续迁移
  continue_on_error: false

# 源环境配置
source:
  mysql:
    endpoints:
      - "source-mysql-host:3306"
    database: "bk_bscp"
    user: "root"
    password: "your_password"
    # 可选连接参数
    dialTimeoutSec: 15
    readTimeoutSec: 30
    writeTimeoutSec: 30
    maxOpenConn: 50
    maxIdleConn: 10
  
  vault:
    address: "http://source-vault:8200"
    token: "your_vault_token"

# 目标环境配置
target:
  mysql:
    endpoints:
      - "target-mysql-host:3306"
    database: "bk_bscp"
    user: "root"
    password: "your_password"
    dialTimeoutSec: 15
    readTimeoutSec: 30
    writeTimeoutSec: 30
    maxOpenConn: 50
    maxIdleConn: 10
  
  vault:
    address: "http://target-vault:8200"
    token: "your_vault_token"

# 跳过的表（运行时/历史数据，通常无需修改）
skip_tables:
  - events
  - current_released_instances
  - resource_locks
  - clients
  - client_events
  - client_querys
  - audits
  - published_strategy_histories
  - archived_apps
  - configs
  - schema_migrations

# 日志配置
log:
  level: "info"
  toStdErr: true
```

### 配置项说明

| 配置项 | 必填 | 默认值 | 说明 |
|--------|------|--------|------|
| `migration.target_tenant_id` | 是 | - | 目标租户 ID |
| `migration.batch_size` | 否 | 1000 | 批量处理大小 |
| `migration.dry_run` | 否 | false | 试运行模式 |
| `migration.continue_on_error` | 否 | false | 遇错继续 |
| `source.mysql.*` | 是 | - | 源 MySQL 连接配置 |
| `target.mysql.*` | 是 | - | 目标 MySQL 连接配置 |
| `source.vault.*` | 否 | - | 源 Vault 配置（迁移 Vault 时必填） |
| `target.vault.*` | 否 | - | 目标 Vault 配置（迁移 Vault 时必填） |

---

## 四、命令使用说明

### 4.1 基本语法

```bash
./bk-bscp-tenant-migration -c <配置文件路径> <命令> [选项]
```

### 4.2 命令列表

| 命令 | 说明 |
|------|------|
| `migrate all` | 完整迁移（MySQL + Vault） |
| `migrate mysql` | 仅迁移 MySQL 数据 |
| `migrate vault` | 仅迁移 Vault KV 数据 |
| `validate` | 验证迁移数据的完整性 |
| `cleanup` | 清理目标数据库中的迁移数据 |
| `version` | 显示版本信息 |

### 4.3 命令详解

#### 完整迁移

```bash
./bk-bscp-tenant-migration -c migration.yaml migrate all
```

按顺序执行：
1. MySQL 数据迁移（按依赖顺序迁移 29 张表）
2. 更新 id_generators 表
3. Vault KV 数据迁移（如已配置）

#### 仅迁移 MySQL

```bash
./bk-bscp-tenant-migration -c migration.yaml migrate mysql
```

适用场景：不使用 Vault 存储 KV 值，或 Vault 单独迁移。

#### 仅迁移 Vault

```bash
./bk-bscp-tenant-migration -c migration.yaml migrate vault
```

> **注意**：Vault 迁移依赖 MySQL 中的 `kvs` 和 `released_kvs` 表记录，请确保 MySQL 数据已迁移。

#### 验证数据

```bash
./bk-bscp-tenant-migration -c migration.yaml validate
```

验证内容：
- 源表与目标表记录数是否一致
- 目标表的 tenant_id 是否正确填充

#### 清理目标数据

```bash
# 交互式确认
./bk-bscp-tenant-migration -c migration.yaml cleanup

# 强制执行（跳过确认）
./bk-bscp-tenant-migration -c migration.yaml cleanup -f
```

> **警告**：此命令会删除目标数据库中所有核心表的数据！仅在迁移失败需要重新执行时使用。

---

## 五、迁移操作流程

### 5.1 推荐流程

```
┌─────────────────────────────────────────────────────────────────┐
│  1. 准备配置文件                                                  │
│     - 填写源/目标环境连接信息                                      │
│     - 设置目标租户 ID                                             │
└───────────────────────────┬─────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. 试运行测试                                                    │
│     设置 dry_run: true 执行迁移命令                                │
│     确认无报错后继续                                               │
└───────────────────────────┬─────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. 执行正式迁移                                                  │
│     设置 dry_run: false                                          │
│     ./bk-bscp-tenant-migration -c config.yaml migrate all        │
└───────────────────────────┬─────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. 验证数据                                                      │
│     ./bk-bscp-tenant-migration -c config.yaml validate           │
└───────────────────────────┬─────────────────────────────────────┘
                            ▼
                    ┌───────────────┐
                    │  验证通过？    │
                    └───────┬───────┘
                   是 │           │ 否
                      ▼           ▼
              ┌──────────┐   ┌──────────────────┐
              │ 迁移完成  │   │ 执行 cleanup     │
              └──────────┘   │ 排查问题后重试    │
                             └──────────────────┘
```

### 5.2 操作示例

```bash
# 步骤 1：试运行测试
# 修改配置文件，设置 dry_run: true
./bk-bscp-tenant-migration -c migration.yaml migrate all

# 步骤 2：正式迁移
# 修改配置文件，设置 dry_run: false
./bk-bscp-tenant-migration -c migration.yaml migrate all

# 步骤 3：验证数据
./bk-bscp-tenant-migration -c migration.yaml validate

# 如果验证失败，清理后重试
./bk-bscp-tenant-migration -c migration.yaml cleanup -f
./bk-bscp-tenant-migration -c migration.yaml migrate all
```

---

## 六、迁移表说明

### 6.1 核心业务表（30 张，按依赖顺序）

**第一层（基础表）**：

| 表名 | 说明 |
|------|------|
| `sharding_bizs` | 业务数据库分片配置 |
| `applications` | 应用定义 |
| `template_spaces` | 模板空间 |
| `groups` | 分组配置 |
| `hooks` | 钩子脚本 |
| `credentials` | 服务密钥凭证 |
| `id_generators` | 主键记录表 |

**第二层（依赖第一层）**：

| 表名 | 说明 |
|------|------|
| `config_items` | 配置项 |
| `releases` | 发布版本 |
| `strategy_sets` | 策略集 |
| `template_sets` | 模板集 |
| `templates` | 模板 |
| `template_variables` | 模板变量 |
| `hook_revisions` | 钩子版本 |
| `credential_scopes` | 凭证作用域 |
| `group_app_binds` | 分组应用绑定 |

**第三层（依赖第二层）**：

| 表名 | 说明 |
|------|------|
| `commits` | 提交记录 |
| `contents` | 配置内容 |
| `strategies` | 发布策略 |
| `current_published_strategies` | 当前生效策略 |
| `kvs` | KV 配置元数据 |
| `released_config_items` | 已发布配置项 |
| `released_groups` | 已发布分组 |
| `released_hooks` | 已发布钩子 |
| `released_kvs` | 已发布 KV |
| `template_revisions` | 模板版本 |
| `app_template_bindings` | 应用模板绑定 |
| `app_template_variables` | 应用模板变量 |
| `released_app_templates` | 已发布应用模板 |
| `released_app_template_variables` | 已发布应用模板变量 |

### 6.2 跳过的表（13 张）

| 表名 | 跳过原因 |
|------|----------|
| `events` | 事件通知，系统自动生成 |
| `current_released_instances` | 客户端重连后自动更新 |
| `resource_locks` | 运行时临时数据 |
| `clients` | 客户端重连后自动注册 |
| `client_events` | 运行时监控数据 |
| `client_querys` | 用户自定义查询 |
| `audits` | 审计日志（可选迁移） |
| `published_strategy_histories` | 发布历史（可选迁移） |
| `archived_apps` | 已归档应用 |
| `configs` | ITSM 配置，新环境重新配置 |
| `schema_migrations` | 数据库迁移记录 |
| `biz_hosts` | 业务主机关系表，定时同步 |
| `sharding_dbs` | 不需要迁移（目标环境会有自己的数据库配置） |

---

## 七、迁移报告说明

### 7.1 迁移报告示例

```
=============================================================
MIGRATION REPORT
=============================================================
Start Time:  2026-01-26T10:00:00+08:00
End Time:    2026-01-26T10:05:30+08:00
Duration:    5m30s
Status:      SUCCESS

MySQL Migration Results:
-------------------------------------------------------------
Table                               Source   Migrated   Status
-------------------------------------------------------------
sharding_bizs                           10         10  SUCCESS
applications                           150        150  SUCCESS
config_items                          1200       1200  SUCCESS
...

Vault Migration Results:
-------------------------------------------------------------
KV Records:          500 migrated of 500
Released KV Records: 1200 migrated of 1200
Status:              SUCCESS

=============================================================
```

### 7.2 验证报告示例

```
=============================================================
VALIDATION REPORT
=============================================================
Start Time:  2026-01-26T10:06:00+08:00
End Time:    2026-01-26T10:06:10+08:00
Duration:    10s
Status:      SUCCESS

Table Validation Results:
-------------------------------------------------------------
Table                          Source     Target    Match TenantID
-------------------------------------------------------------
sharding_bizs                      10         10      YES      YES
applications                      150        150      YES      YES
config_items                     1200       1200      YES      YES
...

=============================================================
```
