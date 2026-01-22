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

package migrator

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/TencentBlueKing/bk-bscp/cmd/tenant-migration/config"
)

// MySQLMigrator handles MySQL data migration
type MySQLMigrator struct {
	cfg      *config.Config
	sourceDB *gorm.DB
	targetDB *gorm.DB
}

// NewMySQLMigrator creates a new MySQLMigrator instance
func NewMySQLMigrator(cfg *config.Config) (*MySQLMigrator, error) {
	// Connect to source database
	sourceDB, err := gorm.Open(mysql.Open(cfg.Source.MySQL.DSN()),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to source database: %w", err)
	}

	sqlDB, err := sourceDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get source database handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(int(cfg.Source.MySQL.MaxOpenConn))
	sqlDB.SetMaxIdleConns(int(cfg.Source.MySQL.MaxIdleConn))

	// Connect to target database
	targetDB, err := gorm.Open(mysql.Open(cfg.Target.MySQL.DSN()),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to target database: %w", err)
	}

	sqlDB, err = targetDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get target database handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(int(cfg.Target.MySQL.MaxOpenConn))
	sqlDB.SetMaxIdleConns(int(cfg.Target.MySQL.MaxIdleConn))

	return &MySQLMigrator{
		cfg:      cfg,
		sourceDB: sourceDB,
		targetDB: targetDB,
	}, nil
}

// Close closes database connections
func (m *MySQLMigrator) Close() error {
	if m.sourceDB != nil {
		sqlDB, err := m.sourceDB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
	if m.targetDB != nil {
		sqlDB, err := m.targetDB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
	return nil
}

// Migrate performs the MySQL data migration
func (m *MySQLMigrator) Migrate() ([]TableMigrationResult, error) {
	coreTables := config.CoreTables()
	results := make([]TableMigrationResult, 0, len(coreTables))

	// Disable foreign key checks on target database
	if err := m.targetDB.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
		return nil, fmt.Errorf("failed to disable foreign key checks: %w", err)
	}
	defer func() {
		if err := m.targetDB.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; err != nil {
			log.Printf("Warning: failed to re-enable foreign key checks: %v", err)
		}
	}()

	for _, tableName := range coreTables {
		if m.cfg.ShouldSkipTable(tableName) {
			log.Printf("Skipping table: %s", tableName)
			continue
		}

		result := m.migrateTable(tableName)
		results = append(results, result)

		if !result.Success && !m.cfg.Migration.ContinueOnError {
			return results, fmt.Errorf("migration failed for table %s: %v", tableName, result.Errors)
		}
	}

	// Update id_generators table
	log.Println("Updating id_generators table...")
	if err := m.updateIDGenerators(); err != nil {
		log.Printf("Warning: failed to update id_generators: %v", err)
	}

	return results, nil
}

// migrateTable migrates a single table
func (m *MySQLMigrator) migrateTable(tableName string) TableMigrationResult {
	startTime := time.Now()
	result := TableMigrationResult{
		TableName: tableName,
		Success:   true,
	}

	log.Printf("Migrating table: %s", tableName)

	// Get source count
	var sourceCount int64
	if err := m.sourceDB.Table(tableName).Count(&sourceCount).Error; err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to count source records: %v", err))
		result.Success = false
		result.Duration = time.Since(startTime)
		return result
	}
	result.SourceCount = sourceCount

	if sourceCount == 0 {
		log.Printf("  Table %s is empty, skipping", tableName)
		result.Duration = time.Since(startTime)
		return result
	}

	// Check if table has tenant_id column
	hasTenantID := m.hasTenantIDColumn(tableName)

	// Migrate in batches
	batchSize := m.cfg.Migration.BatchSize
	offset := 0
	migratedCount := int64(0)

	for {
		var rows []map[string]interface{}
		if err := m.sourceDB.Table(tableName).Offset(offset).Limit(batchSize).Find(&rows).Error; err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to read batch at offset %d: %v", offset, err))
			result.Success = false
			break
		}

		if len(rows) == 0 {
			break
		}

		// Process each row
		for _, row := range rows {
			// Fill tenant_id if the column exists
			if hasTenantID {
				row["tenant_id"] = m.cfg.Migration.TargetTenantID
			}

			// Handle special cases
			m.handleSpecialCases(tableName, row)

			if m.cfg.Migration.DryRun {
				migratedCount++
				continue
			}

			// Insert into target database
			if err := m.targetDB.Table(tableName).Create(row).Error; err != nil {
				result.ErrorCount++
				result.Errors = append(result.Errors, fmt.Sprintf("failed to insert row: %v", err))
				if !m.cfg.Migration.ContinueOnError {
					result.Success = false
					break
				}
			} else {
				migratedCount++
			}
		}

		if !result.Success {
			break
		}

		offset += batchSize
		log.Printf("  Progress: %d/%d records migrated", migratedCount, sourceCount)
	}

	result.MigratedCount = migratedCount
	result.Duration = time.Since(startTime)

	if result.Success {
		log.Printf("  Completed: %d records migrated in %v", migratedCount, result.Duration)
	} else {
		log.Printf("  Failed: %d records migrated, %d errors", migratedCount, result.ErrorCount)
	}

	return result
}

// hasTenantIDColumn checks if a table has tenant_id column
func (m *MySQLMigrator) hasTenantIDColumn(tableName string) bool {
	var count int64
	query := `SELECT COUNT(*) FROM information_schema.columns 
			  WHERE table_schema = ? AND table_name = ? AND column_name = 'tenant_id'`
	if err := m.targetDB.Raw(query, m.cfg.Target.MySQL.Database, tableName).Scan(&count).Error; err != nil {
		log.Printf("Warning: failed to check tenant_id column for table %s: %v", tableName, err)
		return false
	}
	return count > 0
}

// handleSpecialCases handles special type conversions and transformations
func (m *MySQLMigrator) handleSpecialCases(tableName string, row map[string]interface{}) {
	switch tableName {
	case "strategies":
		// itsm_ticket_state_id: int -> string
		if stateID, ok := row["itsm_ticket_state_id"]; ok && stateID != nil {
			switch v := stateID.(type) {
			case int, int32, int64, uint, uint32, uint64:
				row["itsm_ticket_state_id"] = fmt.Sprintf("%v", v)
			case float64:
				row["itsm_ticket_state_id"] = fmt.Sprintf("%.0f", v)
			}
		}
	}
}

// updateIDGenerators updates the id_generators table in target database
func (m *MySQLMigrator) updateIDGenerators() error {
	var generators []struct {
		ID       uint32 `gorm:"column:id"`
		Resource string `gorm:"column:resource"`
		MaxID    uint32 `gorm:"column:max_id"`
	}

	// Read from source
	if err := m.sourceDB.Table("id_generators").Find(&generators).Error; err != nil {
		return fmt.Errorf("failed to read id_generators from source: %w", err)
	}

	// Update target
	for _, g := range generators {
		if m.cfg.Migration.DryRun {
			log.Printf("  Would update id_generators: resource=%s, max_id=%d", g.Resource, g.MaxID)
			continue
		}

		// Check if the resource exists in target
		var count int64
		if err := m.targetDB.Table("id_generators").
			Where("resource = ?", g.Resource).Count(&count).Error; err != nil {
			log.Printf("Warning: failed to check id_generator for resource %s: %v", g.Resource, err)
			continue
		}

		if count > 0 {
			// Update existing record
			if err := m.targetDB.Table("id_generators").
				Where("resource = ?", g.Resource).
				Update("max_id", gorm.Expr("GREATEST(max_id, ?)", g.MaxID)).Error; err != nil {
				log.Printf("Warning: failed to update id_generator for resource %s: %v", g.Resource, err)
			} else {
				log.Printf("  Updated id_generators: resource=%s, max_id=%d", g.Resource, g.MaxID)
			}
		} else {
			// Insert new record
			if err := m.targetDB.Table("id_generators").Create(map[string]interface{}{
				"resource":   g.Resource,
				"max_id":     g.MaxID,
				"updated_at": time.Now(),
			}).Error; err != nil {
				log.Printf("Warning: failed to insert id_generator for resource %s: %v", g.Resource, err)
			} else {
				log.Printf("  Inserted id_generators: resource=%s, max_id=%d", g.Resource, g.MaxID)
			}
		}
	}

	return nil
}

// GetSourceDB returns the source database connection
func (m *MySQLMigrator) GetSourceDB() *gorm.DB {
	return m.sourceDB
}

// GetTargetDB returns the target database connection
func (m *MySQLMigrator) GetTargetDB() *gorm.DB {
	return m.targetDB
}
