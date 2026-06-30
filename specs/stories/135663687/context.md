# Context for Story 135663687

## Stage
validate

## Source artifacts
- specs/stories/135663687/req.md                       # 所有阶段必读（含技术澄清章节）
- specs/stories/135663687/spec.md                      # plan 阶段起必读
- specs/stories/135663687/plan.md                      # tasks 阶段起必读
- specs/stories/135663687/research.md                  # tasks 阶段起必读
- specs/stories/135663687/data-model.md                # tasks 阶段起必读
- specs/stories/135663687/tasks.md                     # implement 阶段起必读
- specs/stories/135663687/validate-arch-report.md       # validate fix 阶段必读（如有）
- specs/stories/135663687/validate-security-report.md   # validate fix 阶段必读（如有）
- specs/stories/135663687/validate-codereview-report.md # validate fix 阶段必读（如有）

## Project background
- AGENTS.md                                             # 用途：仓库协作规则（语言/Go 规范/工作区约束）
- docs/reqs/GSE托管信息检查.md                          # 用途：父需求全文，理解检查侧/操作侧如何使用本表
- pkg/dal/table/process.go                              # 用途：进程表模型（host_id 等 ProcessAttachment 定位字段、枚举风格样板）
- pkg/dal/table/process_instance.go                    # 用途：进程实例表模型（定位字段 tenant/biz/process_id、ProcessStatus/ManagedStatus 枚举样板）
- pkg/dal/table/table.go                               # 用途：表名常量登记位置 + Revision 嵌入结构
- pkg/dal/table/config_template.go                     # 用途：较新的完整 CRUD 表 + 单条 idGen.One 写入样板
- internal/dal/dao/dao.go                              # 用途：Set 接口 + NewDaoSet 工厂挂载方式
- internal/dal/dao/process.go                          # 用途：DAO 接口/实现样板（GetByID/List/BatchCreateWithTx/UpdateSelectedFields）
- internal/dal/dao/process_instance.go                # 用途：DAO 实现样板
- internal/dal/dao/config_template.go                 # 用途：带 idGen.One 单条 Create 的 DAO 样板
- internal/dal/dao/id.go                              # 用途：ID 生成器 IDGenInterface.Batch/One
- internal/dal/dao/set_tenant_id.go                   # 用途：多租户 GORM 回调（tenant_id 自动注入/过滤）
- scripts/gen/main.go                                 # 用途：gorm gen 注册入口（ApplyBasic）+ make gen
- cmd/data-service/db-migration/migrations/20250923114014_add_process.go        # 用途：GormMode 建表 migration 样板（AutoMigrate + id_generators 插入）
- cmd/data-service/db-migration/migrations/20250923114027_add_process_instance.go  # 用途：建表 migration 样板
- cmd/data-service/db-migration/migrator/migrator.go  # 用途：migration 框架与 GormMode/SqlMode
- cmd/data-service/db-migration/README.md             # 用途：migration 创建/执行说明
- pkg/dal/table/table_test.go                         # 用途：table 层单测样式
- bk-process-config-manager/apps/gsekit/process/handlers/check_process.py  # 用途：gsekit error_type/error_msg/handling_suggestion 字段对标
- .cursor/skills/bk-security-redlines/SKILL.md          # 用途：三大安全红线（validate 安全维度）

# 说明：本仓库无 .specify/memory/constitution.md，约束以 AGENTS.md 为准

## Code scope
- cmd/data-service/db-migration/migrations/*_add_process_managed_exception.go  # T001 新增建表 migration
- pkg/dal/table/table.go                               # T002 新增表名常量 ProcessManagedExceptionsTable
- pkg/dal/table/process_managed_exception.go           # T004 新增表模型 + 枚举 + Validate
- pkg/dal/table/process_managed_exception_test.go      # T003 表层单包单测
- scripts/gen/main.go                                  # T005 注册新模型
- internal/dal/gen/**                                  # T005 make gen 生成产物（不手改）
- internal/dal/dao/process_managed_exception.go        # T006/T009/T010/T012/T013/T014 新增 DAO 接口与实现
- internal/dal/dao/dao.go                              # T007 Set 接口挂载工厂方法

## Improvement notes
无
