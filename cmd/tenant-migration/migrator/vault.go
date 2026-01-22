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
	"context"
	"fmt"
	"log"
	"time"

	vault "github.com/hashicorp/vault/api"
	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/cmd/tenant-migration/config"
)

const (
	// MountPath is the Vault mount path for BSCP KV data
	MountPath = "bk_bscp"
	// kvPath is the path format for unreleased KV data
	kvPath = "biz/%d/apps/%d/kvs/%s"
	// releasedKvPath is the path format for released KV data
	releasedKvPath = "biz/%d/apps/%d/releases/%d/kvs/%s"
)

// VaultMigrator handles Vault KV data migration
type VaultMigrator struct {
	cfg         *config.Config
	sourceVault *vault.Client
	targetVault *vault.Client
	mysqlDB     *gorm.DB
}

// KvRecord represents a KV record from MySQL
type KvRecord struct {
	ID      uint32 `gorm:"column:id"`
	BizID   uint32 `gorm:"column:biz_id"`
	AppID   uint32 `gorm:"column:app_id"`
	Key     string `gorm:"column:key"`
	Version uint32 `gorm:"column:version"`
}

// ReleasedKvRecord represents a released KV record from MySQL
type ReleasedKvRecord struct {
	ID        uint32 `gorm:"column:id"`
	BizID     uint32 `gorm:"column:biz_id"`
	AppID     uint32 `gorm:"column:app_id"`
	ReleaseID uint32 `gorm:"column:release_id"`
	Key       string `gorm:"column:key"`
	Version   uint32 `gorm:"column:version"`
}

// NewVaultMigrator creates a new VaultMigrator instance
func NewVaultMigrator(cfg *config.Config, mysqlDB *gorm.DB) (*VaultMigrator, error) {
	// Create source Vault client
	sourceConfig := vault.DefaultConfig()
	sourceConfig.Address = cfg.Source.Vault.Address
	sourceClient, err := vault.NewClient(sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create source Vault client: %w", err)
	}
	sourceClient.SetToken(cfg.Source.Vault.Token)

	// Create target Vault client
	targetConfig := vault.DefaultConfig()
	targetConfig.Address = cfg.Target.Vault.Address
	targetClient, err := vault.NewClient(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create target Vault client: %w", err)
	}
	targetClient.SetToken(cfg.Target.Vault.Token)

	return &VaultMigrator{
		cfg:         cfg,
		sourceVault: sourceClient,
		targetVault: targetClient,
		mysqlDB:     mysqlDB,
	}, nil
}

// Migrate performs the Vault KV data migration
func (m *VaultMigrator) Migrate() (*VaultMigrationResult, error) {
	startTime := time.Now()
	result := &VaultMigrationResult{
		Success: true,
	}

	ctx := context.Background()

	// Step 1: Migrate unreleased KV data
	log.Println("Migrating unreleased KV data...")
	kvRecords, err := m.getKvRecords()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get KV records: %v", err))
		result.Success = false
		result.Duration = time.Since(startTime)
		return result, err
	}
	result.KvCount = int64(len(kvRecords))

	for i, kv := range kvRecords {
		if err := m.migrateKv(ctx, kv); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to migrate KV %d: %v", kv.ID, err))
			if !m.cfg.Migration.ContinueOnError {
				result.Success = false
				result.Duration = time.Since(startTime)
				return result, err
			}
		} else {
			result.MigratedKvs++
		}

		if (i+1)%100 == 0 || i+1 == len(kvRecords) {
			log.Printf("  KV progress: %d/%d migrated", result.MigratedKvs, result.KvCount)
		}
	}

	// Step 2: Migrate released KV data
	log.Println("Migrating released KV data...")
	releasedKvRecords, err := m.getReleasedKvRecords()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get released KV records: %v", err))
		result.Success = false
		result.Duration = time.Since(startTime)
		return result, err
	}
	result.ReleasedKvCount = int64(len(releasedKvRecords))

	for i, rkv := range releasedKvRecords {
		if err := m.migrateReleasedKv(ctx, rkv); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to migrate released KV %d: %v", rkv.ID, err))
			if !m.cfg.Migration.ContinueOnError {
				result.Success = false
				result.Duration = time.Since(startTime)
				return result, err
			}
		} else {
			result.MigratedRKvs++
		}

		if (i+1)%100 == 0 || i+1 == len(releasedKvRecords) {
			log.Printf("  Released KV progress: %d/%d migrated", result.MigratedRKvs, result.ReleasedKvCount)
		}
	}

	result.Duration = time.Since(startTime)
	log.Printf("Vault migration completed: %d KVs, %d released KVs in %v",
		result.MigratedKvs, result.MigratedRKvs, result.Duration)

	return result, nil
}

// getKvRecords retrieves all KV records from MySQL
func (m *VaultMigrator) getKvRecords() ([]KvRecord, error) {
	var records []KvRecord
	if err := m.mysqlDB.Table("kvs").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// getReleasedKvRecords retrieves all released KV records from MySQL
func (m *VaultMigrator) getReleasedKvRecords() ([]ReleasedKvRecord, error) {
	var records []ReleasedKvRecord
	if err := m.mysqlDB.Table("released_kvs").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// migrateKv migrates a single unreleased KV from source to target Vault
func (m *VaultMigrator) migrateKv(ctx context.Context, kv KvRecord) error {
	path := fmt.Sprintf(kvPath, kv.BizID, kv.AppID, kv.Key)

	// Read from source Vault
	secret, err := m.sourceVault.KVv2(MountPath).GetVersion(ctx, path, int(kv.Version))
	if err != nil {
		return fmt.Errorf("failed to read from source Vault: %w", err)
	}

	if secret == nil || secret.Data == nil {
		log.Printf("  Warning: KV %s has no data, skipping", path)
		return nil
	}

	if m.cfg.Migration.DryRun {
		log.Printf("  Would migrate KV: %s", path)
		return nil
	}

	// Write to target Vault
	_, err = m.targetVault.KVv2(MountPath).Put(ctx, path, secret.Data)
	if err != nil {
		return fmt.Errorf("failed to write to target Vault: %w", err)
	}

	return nil
}

// migrateReleasedKv migrates a single released KV from source to target Vault
func (m *VaultMigrator) migrateReleasedKv(ctx context.Context, rkv ReleasedKvRecord) error {
	path := fmt.Sprintf(releasedKvPath, rkv.BizID, rkv.AppID, rkv.ReleaseID, rkv.Key)

	// Read from source Vault
	secret, err := m.sourceVault.KVv2(MountPath).GetVersion(ctx, path, int(rkv.Version))
	if err != nil {
		return fmt.Errorf("failed to read from source Vault: %w", err)
	}

	if secret == nil || secret.Data == nil {
		log.Printf("  Warning: Released KV %s has no data, skipping", path)
		return nil
	}

	if m.cfg.Migration.DryRun {
		log.Printf("  Would migrate released KV: %s", path)
		return nil
	}

	// Write to target Vault
	_, err = m.targetVault.KVv2(MountPath).Put(ctx, path, secret.Data)
	if err != nil {
		return fmt.Errorf("failed to write to target Vault: %w", err)
	}

	return nil
}
