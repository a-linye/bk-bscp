/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package migrations

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	mysqlerr "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/constant"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

// 项目/环境维度迁移的公共逻辑：统一的租户匹配 JOIN、分批回填、扫尾、任务状态管理与 DDL 降级。
// 注意：migrator 在单个事务中执行所有 migration，但 DDL 会隐式提交，失败后依赖各步骤幂等重跑，
// 不要假设存在事务回滚保护。

const (
	scopeStatusPending   = "pending"
	scopeStatusRunning   = "running"
	scopeStatusCompleted = "completed"
	scopeStatusFailed    = "failed"

	scopeTypeProject = "project_scope"
	scopeTypeEnv     = "env_scope"

	backfillBatchSize          = 1000
	backfillBatchSizeForAudits = 200

	// taskStaleTimeout 任务心跳超时，超过该时间未更新的 running 任务视为中断残留，允许重新认领
	taskStaleTimeout = 10 * time.Minute

	// maxLockRetries 批量 UPDATE 遇到死锁/锁等待超时时的最大重试次数
	maxLockRetries = 5

	// progressLogInterval 每处理 N 批输出一行回填进度，便于运维区分正常推进与卡死
	progressLogInterval = 50

	// mysqlErrDeadlock InnoDB 死锁错误码，事务被牺牲后可安全重试
	mysqlErrDeadlock = 1213
	// mysqlErrLockWaitTimeout InnoDB 锁等待超时错误码，重试可等待锁释放
	mysqlErrLockWaitTimeout = 1205
	// mysqlErrUnsupportedAlter ALGORITHM/LOCK 子句不被当前 MySQL 版本或引擎支持
	mysqlErrUnsupportedAlter = 1845
	// mysqlErrUnsupportedAlterReason 同 1845，携带具体不支持原因的变体
	mysqlErrUnsupportedAlterReason = 1846
)

// tableIdentPattern 表名白名单校验，防止标识符拼接引入 SQL 注入（表名无法参数化）
var tableIdentPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// envScopeTableSet 环境维度表集合，用于决定回填 SQL 是否需要 JOIN environments
var envScopeTableSet = map[string]bool{
	"applications": true, "client_querys": true, "clients": true, "credential_scopes": true, "events": true,
}

type batchIDs struct {
	ID uint64 `gorm:"column:id"`
}

func quoteTable(name string) string {
	return "`" + name + "`"
}

// tenantNorm 租户归一化表达式，兼容历史数据中 tenant_id 为 NULL 或空字符串的场景
func tenantNorm(alias string) string {
	return fmt.Sprintf("CASE WHEN %[1]s.tenant_id IS NULL OR %[1]s.tenant_id = '' THEN 'default' ELSE %[1]s.tenant_id END", alias)
}

// joinDefaultProject 生成按 biz_id + 归一化租户匹配默认项目的 JOIN 子句
func joinDefaultProject(t, p string) string {
	return fmt.Sprintf("INNER JOIN projects %s ON %s.biz_id = %s.biz_id AND %s = %s AND %s.is_default = 1",
		p, p, t, tenantNorm(p), tenantNorm(t), p)
}

// joinDefaultEnv 生成匹配默认环境（prod 类型、内置默认名）的 JOIN 子句
func joinDefaultEnv(p, e string) string {
	return fmt.Sprintf("INNER JOIN environments %s ON %s.project_id = %s.id AND %s = %s AND %s.type = 'prod' AND %s.name = '%s'",
		e, e, p, tenantNorm(e), tenantNorm(p), e, e, table.DefaultEnvName)
}

// scopeNullCondition 维度缺失判定条件（含 0 值，兼容回填期间的异常写入）
func scopeNullCondition(tableName string) string {
	if envScopeTableSet[tableName] {
		return "(t.project_id IS NULL OR t.project_id = 0 OR t.environment_id IS NULL OR t.environment_id = 0)"
	}
	return "(t.project_id IS NULL OR t.project_id = 0)"
}

// assertScopeFilled 断言维度已全部回填。回填基于 INNER JOIN，匹配不上默认项目的行
// 会被静默跳过（UPDATE 影响 0 行不报错），而 MySQL 唯一索引中 NULL ≠ NULL，
// 若带着残留 NULL 行删除旧唯一索引，这些行将永久失去唯一性约束且不会自愈，
// 因此必须在切换唯一索引前把静默残留变成一次明确的迁移失败。
// config_delivery 等业务级共享空间例外：不归属任何项目，但必须恰好是 0，
// 残留 NULL 同样会让即将创建的唯一索引对该行失效，必须在此拦下。
func assertScopeFilled(db *gorm.DB, tableName string) error {
	if !tableIdentPattern.MatchString(tableName) {
		return fmt.Errorf("invalid table name %q", tableName)
	}

	cond := scopeNullCondition(tableName)
	if tableName == "template_spaces" {
		cond = fmt.Sprintf(
			"((t.name <> '%s' AND %s) OR (t.name = '%s' AND (t.project_id IS NULL OR t.project_id <> %d)))",
			constant.CONFIG_DELIVERY, cond, constant.CONFIG_DELIVERY, constant.GlobalProjectID)
	}
	var cnt int64
	sql := fmt.Sprintf("SELECT COUNT(*) FROM %s t WHERE %s", quoteTable(tableName), cond)
	if err := db.Raw(sql).Scan(&cnt).Error; err != nil {
		return fmt.Errorf("count unfilled rows in %s failed: %w", tableName, err)
	}
	if cnt > 0 {
		return fmt.Errorf("table %s still has %d rows without project/environment, refuse to switch unique index", tableName, cnt)
	}
	return nil
}

// normalizeGlobalTemplateSpaces 将业务级共享的内置模板空间显式置为 0。
// 不依赖 JOIN（默认项目缺失的业务也能修正），必须在切换唯一索引之前完成：
// 残留 NULL 会因唯一索引中 NULL ≠ NULL 而永久失去业务内唯一性。
// 若存量数据已存在重复行，置 0 会直接撞唯一键失败，属预期的显式中止。
func normalizeGlobalTemplateSpaces(db *gorm.DB) error {
	return db.Exec(
		"UPDATE `template_spaces` SET project_id = ?, updated_at = ? WHERE name = ? AND (project_id IS NULL OR project_id <> ?)",
		constant.GlobalProjectID, time.Now().UTC(), constant.CONFIG_DELIVERY, constant.GlobalProjectID,
	).Error
}

// buildFillUpdateSQL 构造单批回填 UPDATE，统一带租户归一化匹配
func buildFillUpdateSQL(tableName string) string {
	if tableName == "template_spaces" {
		// config_delivery 为系统内置模板空间，业务级共享不归属任何项目，
		// 由 normalizeGlobalTemplateSpaces 统一置为 0，回填/扫尾按名称排除防止被改成默认项目 id。
		// 注意 Go 侧 uint32 零值写入的是 0 而非 NULL，缺失判定必须同时覆盖 NULL 和 0
		return fmt.Sprintf(`UPDATE %s t %s
			SET t.project_id = p.id
			WHERE t.id IN (?) AND t.name <> 'config_delivery' AND (t.project_id IS NULL OR t.project_id = 0)`,
			quoteTable(tableName), joinDefaultProject("t", "p"))
	}

	if envScopeTableSet[tableName] {
		setClause := "t.project_id = p.id, t.environment_id = e.id"
		if tableName == "applications" {
			setClause += ", t.env_display = CONCAT(e.name, '-', e.type)"
		}
		return fmt.Sprintf(`UPDATE %s t %s %s SET %s WHERE t.id IN (?) AND %s`,
			quoteTable(tableName), joinDefaultProject("t", "p"), joinDefaultEnv("p", "e"), setClause,
			scopeNullCondition(tableName))
	}

	return fmt.Sprintf(`UPDATE %s t %s SET t.project_id = p.id WHERE t.id IN (?) AND %s`,
		quoteTable(tableName), joinDefaultProject("t", "p"), scopeNullCondition(tableName))
}

// isRetryableLockError 判断是否为可重试的死锁或锁等待超时
func isRetryableLockError(err error) bool {
	var myErr *mysqlerr.MySQLError
	if errors.As(err, &myErr) {
		return myErr.Number == mysqlErrDeadlock || myErr.Number == mysqlErrLockWaitTimeout
	}
	return false
}

// execWithRetry 执行 DML，遇到死锁/锁等待超时时按递增间隔重试；末次失败直接返回，
// 重试等待可被 session context 取消，避免进程退出时被 sleep 阻塞
func execWithRetry(db *gorm.DB, sql string, args ...interface{}) error {
	ctx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		ctx = db.Statement.Context
	}

	var err error
	for attempt := 0; attempt < maxLockRetries; attempt++ {
		if err = db.Exec(sql, args...).Error; err == nil {
			return nil
		}
		if !isRetryableLockError(err) || attempt == maxLockRetries-1 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(200*(attempt+1)) * time.Millisecond):
		}
	}
	return err
}

// execDDLWithFallback 优先使用 INSTANT/INPLACE LOCK=NONE 的低风险 DDL，
// 仅在 MySQL 明确报「算法/锁子句不支持」（1845/1846）时降级为普通 DDL
// （低版本 MySQL < 8.0.12 等），其他错误如实返回不吞掉。
func execDDLWithFallback(db *gorm.DB, fastDDL, plainDDL string) error {
	err := db.Exec(fastDDL).Error
	if err == nil {
		return nil
	}

	var myErr *mysqlerr.MySQLError
	if !errors.As(err, &myErr) ||
		(myErr.Number != mysqlErrUnsupportedAlter && myErr.Number != mysqlErrUnsupportedAlterReason) {
		return err
	}

	// 降级会退回可能重建全表的阻塞 DDL，必须打印醒目告警提示运维评估锁表影响
	fmt.Printf("WARNING: online ddl unsupported (%v), falling back to blocking ddl "+
		"which may rebuild the whole table: %s\n", err, plainDDL)
	return db.Exec(plainDDL).Error
}

// getStandaloneDB 基于外层事务的连接池构造独立会话，避免长时间回填占用事务连接。
// 直接用已有连接池构造 dialector，避免 gorm.Open 重复建池导致连接泄漏。
func getStandaloneDB(tx *gorm.DB) (*gorm.DB, error) {
	rawSQLDB, err := tx.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	standaloneDB, err := gorm.Open(mysql.New(mysql.Config{Conn: rawSQLDB}), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 tx.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open standalone gorm session: %w", err)
	}
	return standaloneDB.WithContext(tx.Statement.Context), nil
}

// claimMigrationTask 抢占式认领回填任务，保证并发/重试场景下同一表只有一个执行者。
// 返回 claimed=false 表示其他实例正在执行，调用方应跳过（由扫尾阶段兜底）。
func claimMigrationTask(db *gorm.DB, tableName, scopeType string) (*ScopeMigrationTask, bool, error) {
	now := time.Now()
	if err := db.Exec(`INSERT IGNORE INTO scope_migration_tasks (table_name, scope_type, last_id, status, updated_at) VALUES (?, ?, 0, ?, ?)`,
		tableName, scopeType, scopeStatusPending, now).Error; err != nil {
		return nil, false, fmt.Errorf("init migration task for %s failed: %w", tableName, err)
	}

	var task ScopeMigrationTask
	if err := db.Where("table_name = ?", tableName).Take(&task).Error; err != nil {
		return nil, false, fmt.Errorf("load migration task for %s failed: %w", tableName, err)
	}
	if task.Status == scopeStatusCompleted {
		return &task, false, nil
	}

	// 非 running 状态直接认领；running 但心跳超时的视为中断残留，允许重新认领
	res := db.Exec(`UPDATE scope_migration_tasks SET status = ?, error_msg = '', updated_at = ? 
		WHERE table_name = ? AND (status <> ? OR updated_at < ?)`,
		scopeStatusRunning, now, tableName, scopeStatusRunning, now.Add(-taskStaleTimeout))
	if res.Error != nil {
		return nil, false, fmt.Errorf("claim migration task for %s failed: %w", tableName, res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	return &task, true, nil
}

func updateTaskProgress(db *gorm.DB, tableName string, lastID uint64) error {
	return db.Exec(`UPDATE scope_migration_tasks SET last_id = ?, updated_at = ? WHERE table_name = ?`,
		lastID, time.Now(), tableName).Error
}

func completeMigrationTask(db *gorm.DB, tableName string, lastID uint64) error {
	return db.Exec(`UPDATE scope_migration_tasks SET last_id = ?, status = ?, error_msg = '', updated_at = ? WHERE table_name = ?`,
		lastID, scopeStatusCompleted, time.Now(), tableName).Error
}

func failMigrationTask(db *gorm.DB, tableName string, lastID uint64, errMsg string) error {
	return db.Exec(`UPDATE scope_migration_tasks SET last_id = ?, status = ?, error_msg = ?, updated_at = ? WHERE table_name = ?`,
		lastID, scopeStatusFailed, errMsg, time.Now(), tableName).Error
}

// resetMigrationTask 删除任务记录，使 Down 之后重新 Up 时能够重新执行回填
func resetMigrationTask(db *gorm.DB, tableName string) error {
	if err := db.Exec("DELETE FROM scope_migration_tasks WHERE table_name = ?", tableName).Error; err != nil {
		return fmt.Errorf("reset migration task for %s failed: %w", tableName, err)
	}
	return nil
}

// fillScopeDimension 按 id 游标分批回填维度列，每批独立提交且有死锁重试，控制锁粒度。
// onlyNull 为 true 时仅扫描维度缺失的行（扫尾模式）；游标严格递增，即使存在无法匹配
// 默认项目的行也能保证终止。
func fillScopeDimension(db *gorm.DB, tableName string, batchSize int, lastID uint64,
	onlyNull bool, onBatch func(maxID uint64) error) (uint64, error) {

	if !tableIdentPattern.MatchString(tableName) {
		return lastID, fmt.Errorf("invalid table name %q", tableName)
	}

	nullFilter := ""
	if onlyNull {
		nullFilter = "AND " + scopeNullCondition(tableName)
	}
	updateSQL := buildFillUpdateSQL(tableName)

	start := time.Now()
	batchCount, processed := 0, 0

	for {
		var ids []batchIDs
		selSQL := fmt.Sprintf("SELECT t.id FROM %s t WHERE t.id > ? %s ORDER BY t.id LIMIT %d",
			quoteTable(tableName), nullFilter, batchSize)
		if err := db.Raw(selSQL, lastID).Scan(&ids).Error; err != nil {
			return lastID, fmt.Errorf("select batch ids from %s failed: %w", tableName, err)
		}
		if len(ids) == 0 {
			fmt.Printf("scope backfill %s done: %d rows in %d batches, cost %s\n",
				tableName, processed, batchCount, time.Since(start).Round(time.Millisecond))
			return lastID, nil
		}

		idList := make([]uint64, len(ids))
		for i, r := range ids {
			idList[i] = r.ID
		}
		maxIDInBatch := idList[len(idList)-1]

		if err := execWithRetry(db, updateSQL, idList); err != nil {
			return lastID, fmt.Errorf("fill %s batch (%d-%d] failed: %w", tableName, lastID, maxIDInBatch, err)
		}

		lastID = maxIDInBatch
		batchCount++
		processed += len(idList)
		if batchCount%progressLogInterval == 0 {
			fmt.Printf("scope backfill %s progress: cursor=%d, processed=%d rows, cost %s\n",
				tableName, lastID, processed, time.Since(start).Round(time.Millisecond))
		}
		if onBatch != nil {
			if err := onBatch(lastID); err != nil {
				return lastID, fmt.Errorf("update progress for %s failed: %w", tableName, err)
			}
		}
	}
}

// backfillScopeTable 带任务状态的分批回填，支持断点续跑。
// 注意：认领机制只保护回填阶段本身——抢不到任务时直接返回，本实例继续执行
// 扫尾与索引切换。并发实例同时切索引由 HasIndex 幂等守卫去重；切唯一索引前的
// assertScopeFilled 断言把残留行（含对方尚未回填完的）变成明确的迁移失败，
// 因此无需为索引切换阶段引入额外的互斥。
func backfillScopeTable(db *gorm.DB, tableName, scopeType string, batchSize int) error {
	task, claimed, err := claimMigrationTask(db, tableName, scopeType)
	if err != nil {
		return err
	}
	if !claimed {
		// 已完成或其他实例正在执行回填，跳过回填；
		// 若对方未回填完，本实例的扫尾会补一部分，切唯一索引前由断言拦截
		return nil
	}

	lastID, err := fillScopeDimension(db, tableName, batchSize, task.LastID, false, func(maxID uint64) error {
		return updateTaskProgress(db, tableName, maxID)
	})
	if err != nil {
		_ = failMigrationTask(db, tableName, lastID, err.Error())
		return err
	}
	return completeMigrationTask(db, tableName, lastID)
}

func backfillProjectScopeTable(db *gorm.DB, tableName string) error {
	batchSize := backfillBatchSize
	if tableName == "audits" {
		batchSize = backfillBatchSizeForAudits
	}
	return backfillScopeTable(db, tableName, scopeTypeProject, batchSize)
}

func backfillEnvScopeTable(db *gorm.DB, tableName string) error {
	return backfillScopeTable(db, tableName, scopeTypeEnv, backfillBatchSize)
}

// sweepProjectScopeTable 分批扫尾，补偿回填窗口期并发写入导致的维度缺失行
// （含 Go 侧 uint32 零值写入的 0）。template_spaces 的 config_delivery 行会被
// SELECT 选中但 UPDATE 按名称排除不匹配，游标照常递增，循环可正常终止。
func sweepProjectScopeTable(db *gorm.DB, tableName string) error {
	batchSize := backfillBatchSize
	if tableName == "audits" {
		batchSize = backfillBatchSizeForAudits
	}
	_, err := fillScopeDimension(db, tableName, batchSize, 0, true, nil)
	return err
}

func sweepEnvScopeTable(db *gorm.DB, tableName string) error {
	_, err := fillScopeDimension(db, tableName, backfillBatchSize, 0, true, nil)
	return err
}
