# Context for Story 135663906

## Stage
tasks

## Source artifacts
- specs/stories/135663906/req.md                       # 所有阶段必读（含技术澄清章节）
- specs/stories/135663906/spec.md                      # plan 阶段起必读
- specs/stories/135663906/questions.md                 # Q-001~Q-007 澄清结论（resolved_by_doc）
- specs/stories/135663906/plan.md                      # tasks 阶段起必读（文件级改动 + TDD 步骤）
- specs/stories/135663906/research.md                  # tasks 阶段起必读（D1~D11 技术决策）
- specs/stories/135663906/data-model.md                # tasks 阶段起必读（运行态结构 + 比对/分类规则）

## Project background
- AGENTS.md                                            # 用途：仓库协作规则（语言/Go 规范/工作区约束）
- docs/reqs/GSE托管信息检查.md                         # 用途：父需求全文，理解检查侧整体设计与异常类别
- cmd/data-service/app/app.go                          # 用途：crontab 任务启动入口 startCronTasks()/initTaskManager()
- cmd/data-service/service/crontab/sync_cmdb.go        # 用途：周期任务样板（20min 周期 + IsMaster 守卫 + 按租户遍历 + rateLimiter 字段）
- cmd/data-service/service/crontab/sync_biz_host.go    # 用途：rate.NewLimiter + rateLimiter.Wait() 限流用法样板
- pkg/cc/types.go                                      # 用途：CrontabConfig/SyncCmdbGseConfig 配置结构体定义位置
- pkg/cc/service.go                                    # 用途：data-service 配置加载 + crontab 默认值/校验
- cmd/data-service/etc/data_service.yaml               # 用途：crontab 开关/周期/QPS 配置样例
- internal/serviced/serviced.go                        # 用途：State.IsMaster() 主从选举接口与实现
- internal/task/executor/config/config_check.go        # 用途：配置检查机制核心执行器（GSE 异步脚本 + 结果解析）
- internal/task/executor/common/common.go              # 用途：WaitExecuteScriptFinish 轮询封装（GetExecuteScriptResult）
- internal/components/gse/script.go                    # 用途：GSE API 封装 AsyncExtensionsExecuteScript/GetExecuteScriptResult
- internal/components/gse/type.go                      # 用途：GSE 请求/响应类型（ExecuteScriptReq/ExecuteScriptResult 等）
- pkg/dal/table/process.go                             # 用途：ProcessInfo 期望托管配置字段 + Process/ProcessSpec.SourceData JSON
- pkg/dal/table/process_instance.go                    # 用途：ProcessInstance 表模型（Status/ManagedStatus 等）
- internal/dal/dao/process.go                          # 用途：Process DAO（List/ListProcessesWithInstance 等）
- internal/dal/dao/process_instance.go                 # 用途：ProcessInstance DAO（GetByProcessIDs/BatchUpdate 等）
- internal/processor/gse/sync_gse.go                   # 用途：SyncSingleBiz 按业务遍历进程实例 + 批次并发样板
- internal/processor/gse/gse.go                        # 用途：BuildProcessOperate 的 namespace/valuekey 构造（GSEKIT_BIZ_/alias_hostInstSeq）
- internal/dal/dao/app.go                              # 用途：ListBizTenantMap（biz→tenant 跨租户遍历）
- internal/components/bkuser/bkuser.go                 # 用途：ListEnabledTenants 启用租户列表
- specs/stories/135663687/spec.md                      # 用途：上游强依赖需求规范，理解异常记录表/DAO 契约
- specs/stories/135663687/data-model.md                # 用途：上游异常记录数据模型（字段/枚举/状态机）
- pkg/dal/table/process_managed_exception.go           # 用途：上游异常记录表模型 + 异常类型/状态枚举（复用）
- internal/dal/dao/process_managed_exception.go        # 用途：上游异常记录 DAO（Create/GetLatest/IsException/UpdateStatus）
- bk-process-config-manager/apps/gsekit/process/handlers/check_process.py  # 用途：gsekit 异常类别/error_type 对标
- .cursor/skills/bk-security-redlines/SKILL.md         # 用途：三大安全红线（validate 安全维度）

# 说明：本仓库无 .specify/memory/constitution.md，约束以 AGENTS.md 为准。
# CodeReview 维度由 code-reviewer agent 内置清单驱动，无需在白名单引入 .cursor/skills/code-review/。

## Code scope
- pkg/cc/types.go                                      # 新增 CheckProcessManagedConfig + 挂到 CrontabConfig + trySetDefault/validate
- pkg/cc/service.go                                    # 若 crontab 默认/校验在此则串接（与既有 crontab 一致处）
- cmd/data-service/etc/data_service.yaml               # crontab 下新增 checkProcessManaged 样例（enabled:false 缺省）
- internal/processor/processcheck/**                   # 新增核心检查包（parse/compare/expected/record/executor/checker + 单测）
- cmd/data-service/service/crontab/check_managed_process.go  # 新增定时任务入口（ticker+shutdown+IsMaster+跨业务遍历+限流）
- cmd/data-service/app/app.go                          # startCronTasks() 按 Enabled 守卫启动新巡检任务（注入 daoSet/sd/gseSvc）

## Improvement notes
无
