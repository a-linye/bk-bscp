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
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/cmd/data-service/db-migration/migrator"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

func init() {
	// add current migration to migrator
	migrator.GetMigrator().AddMigration(&migrator.Migration{
		Version: "20260817152329",
		Name:    "20260817152329_init_projects_and_environments",
		Mode:    migrator.GormMode,
		Up:      mig20260817152329Up,
		Down:    mig20260817152329Down,
	})
}

// =============================================
// 数据结构定义
// =============================================

type Project struct {
	ID        uint32    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id;type:varchar(255);not null;default:'default'"`
	BizID     uint32    `gorm:"column:biz_id;not null"`
	Key       string    `gorm:"column:key;size:64;not null"`
	Name      string    `gorm:"column:name;size:255;not null"`
	Memo      string    `gorm:"column:memo;size:256"`
	Protected bool      `gorm:"column:protected;not null;default:false"`
	IsDefault *bool     `gorm:"column:is_default;type:tinyint(1);default:null"`
	Creator   string    `gorm:"column:creator;size:64;not null"`
	Reviser   string    `gorm:"column:reviser;size:64;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (Project) TableName() string { return "projects" }

type Environment struct {
	ID           uint32    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id;type:varchar(255);not null;default:'default'"`
	BizID        uint32    `gorm:"column:biz_id;not null"`
	ProjectID    uint32    `gorm:"column:project_id;not null"`
	Name         string    `gorm:"column:name;size:255;not null"`
	Type         string    `gorm:"column:type;size:20;not null"`
	Memo         string    `gorm:"column:memo;size:256"`
	DisplayOrder int       `gorm:"column:display_order;type:int;not null;default:0"`
	Protected    bool      `gorm:"column:protected;not null;default:false"`
	Creator      string    `gorm:"column:creator;size:64;not null"`
	Reviser      string    `gorm:"column:reviser;size:64;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

func (Environment) TableName() string { return "environments" }

type ScopeMigrationTask struct {
	TargetTable string    `gorm:"column:table_name;type:varchar(128);not null;primaryKey"`
	ScopeType   string    `gorm:"column:scope_type;type:varchar(32);not null"`
	LastID      uint64    `gorm:"column:last_id;type:bigint(20) unsigned;not null;default:0"`
	Status      string    `gorm:"column:status;type:varchar(32);not null;default:'pending'"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
	ErrorMsg    string    `gorm:"column:error_msg;type:text"`
}

func (ScopeMigrationTask) TableName() string { return "scope_migration_tasks" }

// mig20260817152329Up for up migration
func mig20260817152329Up(tx *gorm.DB) error {
	// 基础结构准备
	if err := stepCreateProjectEnvTables(tx); err != nil {
		return fmt.Errorf("step1 create project/env tables failed: %w", err)
	}
	// 插入默认项目与环境（检查是否存在，避免重试绑错）
	if err := stepInsertDefaultProjectsAndEnvs(tx); err != nil {
		return fmt.Errorf("insert default projects/envs failed: %w", err)
	}

	return nil
}

// mig20260817152329Down for down migration
func mig20260817152329Down(tx *gorm.DB) error {

	for _, t := range []string{"environments", "projects", "scope_migration_tasks"} {
		if tx.Migrator().HasTable(t) {
			if err := tx.Migrator().DropTable(t); err != nil {
				return fmt.Errorf("drop %s table failed: %w", t, err)
			}
		}
	}

	if err := tx.Exec("DELETE FROM id_generators WHERE resource IN ('projects', 'environments')").Error; err != nil {
		return fmt.Errorf("clean id_generators failed: %w", err)
	}

	return nil
}

// 建表与注册 id_generators
func stepCreateProjectEnvTables(tx *gorm.DB) error {
	const tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"

	if err := tx.Set("gorm:table_options", tableOptions).AutoMigrate(&Project{}); err != nil {
		return fmt.Errorf("create projects table: %w", err)
	}

	if !tx.Migrator().HasIndex("projects", "idx_key") {
		if err := tx.Exec("CREATE UNIQUE INDEX idx_key ON projects (`key`)").Error; err != nil {
			return fmt.Errorf("create idx_key on projects: %w", err)
		}
	}
	if !tx.Migrator().HasIndex("projects", "uk_tenantID_bizID_isDefault") {
		if err := tx.Exec("CREATE UNIQUE INDEX uk_tenantID_bizID_isDefault ON projects (tenant_id, biz_id, is_default)").Error; err != nil {
			return fmt.Errorf("create uk_tenantID_bizID_isDefault on projects: %w", err)
		}
	}

	if err := tx.Set("gorm:table_options", tableOptions).AutoMigrate(&Environment{}); err != nil {
		return fmt.Errorf("create environments table: %w", err)
	}

	if !tx.Migrator().HasIndex("environments", "idx_tenantID_bizID_projectID_name") {
		if err := tx.Exec("CREATE UNIQUE INDEX idx_tenantID_bizID_projectID_name ON environments (tenant_id, biz_id, project_id, name)").Error; err != nil {
			return fmt.Errorf("create index on environments: %w", err)
		}
	}

	if err := tx.Set("gorm:table_options", tableOptions).AutoMigrate(&ScopeMigrationTask{}); err != nil {
		return fmt.Errorf("create scope_migration_tasks table: %w", err)
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	if err := tx.Exec("INSERT IGNORE INTO id_generators(resource, max_id, updated_at) VALUES ('projects', 0, ?)", now).Error; err != nil {
		return fmt.Errorf("register projects id_generator: %w", err)
	}
	if err := tx.Exec("INSERT IGNORE INTO id_generators(resource, max_id, updated_at) VALUES ('environments', 0, ?)", now).Error; err != nil {
		return fmt.Errorf("register environments id_generator: %w", err)
	}

	return nil
}

type BizTenantRecord struct {
	BizID    uint64 `gorm:"column:biz_id"`
	TenantID string `gorm:"column:tenant_id"`
}

// 插入默认项目与环境
func stepInsertDefaultProjectsAndEnvs(tx *gorm.DB) error {
	// 扫描所有现有业务表（applications、hooks、credentials等），归纳出系统中所有出现过的 biz_id（业务ID）
	allTables := []string{"applications", "hooks", "groups", "credentials", "template_spaces", "template_variables", "audits", "clients", "events"}
	var unionSQLs []string
	for _, tbl := range allTables {
		unionSQLs = append(unionSQLs, fmt.Sprintf(
			`SELECT DISTINCT biz_id, CASE WHEN tenant_id IS NULL OR tenant_id = '' THEN 'default' ELSE tenant_id END as tenant_id FROM %s WHERE biz_id > 0`,
			quoteTable(tbl),
		))
	}
	finalSQL := fmt.Sprintf("SELECT DISTINCT biz_id, tenant_id FROM (%s) AS all_resources", strings.Join(unionSQLs, " UNION "))

	var bizTenantList []BizTenantRecord
	if err := tx.Raw(finalSQL).Scan(&bizTenantList).Error; err != nil {
		return fmt.Errorf("query unique biz/tenant failed: %w", err)
	}
	if len(bizTenantList) == 0 {
		return nil
	}

	now := time.Now()
	systemUser := table.System

	for _, bt := range bizTenantList {
		bizID := uint32(bt.BizID)
		tenantID := bt.TenantID

		// 1. 查询已有默认项目
		var actualProjID uint64
		err := tx.Raw("SELECT id FROM projects WHERE tenant_id = ? AND biz_id = ? AND is_default = 1 LIMIT 1", tenantID, bizID).
			Scan(&actualProjID).Error
		if err != nil {
			return fmt.Errorf("check existing project for biz %d: %w", bizID, err)
		}

		if actualProjID == 0 {
			// 未找到时，安全自增取号
			var projGen struct{ MaxID uint64 }
			if err := tx.Raw("SELECT max_id FROM id_generators WHERE resource = 'projects' FOR UPDATE").Scan(&projGen).Error; err != nil {
				return fmt.Errorf("lock projects id_generator failed: %w", err)
			}
			allocProjID := projGen.MaxID + 1
			defaultProjectKey := table.GenerateProjectKey(uint32(allocProjID))

			res := tx.Exec(
				`INSERT IGNORE INTO projects (id, tenant_id, biz_id, `+"`key`"+`, name, memo, `+
					`protected, is_default, creator, reviser, created_at, updated_at) `+
					`VALUES (?, ?, ?, ?, ?, '', true, 1, ?, ?, ?, ?)`,
				allocProjID, tenantID, bizID, defaultProjectKey, table.DefaultProjectName, systemUser, systemUser, now, now,
			)
			if res.Error != nil {
				return fmt.Errorf("insert project for biz %d: %w", bizID, res.Error)
			}
			if res.RowsAffected == 0 {
				// 并发迁移或重试已在预检查之后插入同一默认项目（唯一键 tenant_id+biz_id+is_default 冲突），
				// 重新查询其实际 id 继续，避免后续环境挂到未插入的 id 上。
				// 注意必须用锁定读：事务快照可能看不到其他事务后提交的冲突行。
				if err := tx.Raw("SELECT id FROM projects WHERE tenant_id = ? AND biz_id = ? AND is_default = 1 LIMIT 1 FOR UPDATE",
					tenantID, bizID).Scan(&actualProjID).Error; err != nil {
					return fmt.Errorf("re-check existing project for biz %d: %w", bizID, err)
				}
				if actualProjID == 0 {
					// 重查仍不存在说明是主键/key 真冲突，生成器已落后，必须中止迁移
					return fmt.Errorf("insert project for biz %d affected 0 rows and default project not found, id/key conflict on id=%d",
						bizID, allocProjID)
				}
			} else {
				// 插入成功，后续默认环境必须挂到本次分配的项目 id 上
				actualProjID = allocProjID
				// 带单调守卫：迁移期间应用侧可能已创建更大 id 的项目，避免把 max_id 改小导致取号撞主键
				if err := tx.Exec("UPDATE id_generators SET max_id = ?, updated_at = ? WHERE resource = 'projects' AND max_id < ?",
					allocProjID, now, allocProjID).Error; err != nil {
					// 生成器推进失败，继续执行会导致后续取号撞主键，必须中止
					return fmt.Errorf("update projects id_generator to %d failed: %w", allocProjID, err)
				}
			}
		}

		// 2. 检查并补全默认环境
		var existingEnvID uint64
		if err := tx.Raw(
			"SELECT id FROM environments WHERE tenant_id = ? AND biz_id = ? AND project_id = ? AND name = ? LIMIT 1",
			tenantID, bizID, actualProjID, table.DefaultEnvName,
		).Scan(&existingEnvID).Error; err != nil {
			// 查询失败会误判为默认环境不存在，进而走取号逻辑并最终报误导性的唯一键冲突，
			// 必须在此中止并暴露真实错误
			return fmt.Errorf("check existing environment for biz %d: %w", bizID, err)
		}

		if existingEnvID == 0 {
			var envGen struct{ MaxID uint64 }
			if err := tx.Raw("SELECT max_id FROM id_generators WHERE resource = 'environments' FOR UPDATE").Scan(&envGen).Error; err != nil {
				return fmt.Errorf("lock environments id_generator failed: %w", err)
			}
			nextEnvID := envGen.MaxID + 1

			resE := tx.Exec(
				`INSERT IGNORE INTO environments (id, tenant_id, biz_id, project_id, name, `+
					`type, memo, display_order, protected, creator, reviser, created_at, updated_at) `+
					`VALUES (?, ?, ?, ?, ?, 'prod', '', 0, true, ?, ?, ?, ?)`,
				nextEnvID, tenantID, bizID, actualProjID, table.DefaultEnvName, systemUser, systemUser, now, now,
			)
			if resE.Error != nil {
				return fmt.Errorf("insert environment for biz %d: %w", bizID, resE.Error)
			}
			if resE.RowsAffected == 0 {
				// 并发迁移或重试已在预检查之后插入同一默认环境（唯一键冲突），
				// 锁定读重查确认其存在即可继续；事务快照可能看不到其他事务后提交的冲突行。
				var reEnvID uint64
				if err := tx.Raw("SELECT id FROM environments WHERE tenant_id = ? AND biz_id = ? AND project_id = ? AND name = ? LIMIT 1 FOR UPDATE",
					tenantID, bizID, actualProjID, table.DefaultEnvName).Scan(&reEnvID).Error; err != nil {
					return fmt.Errorf("re-check existing environment for biz %d: %w", bizID, err)
				}
				if reEnvID == 0 {
					// 重查仍不存在说明是主键真冲突，生成器已落后，必须中止迁移
					return fmt.Errorf("insert environment for biz %d affected 0 rows and default environment not found, id conflict on id=%d",
						bizID, nextEnvID)
				}
			} else {
				// 带单调守卫：迁移期间应用侧可能已创建更大 id 的环境，避免把 max_id 改小导致取号撞主键
				if err := tx.Exec("UPDATE id_generators SET max_id = ?, updated_at = ? WHERE resource = 'environments' AND max_id < ?",
					nextEnvID, now, nextEnvID).Error; err != nil {
					// 生成器推进失败，继续执行会导致后续取号撞主键，必须中止
					return fmt.Errorf("update environments id_generator to %d failed: %w", nextEnvID, err)
				}
			}
		}
	}

	return nil
}
