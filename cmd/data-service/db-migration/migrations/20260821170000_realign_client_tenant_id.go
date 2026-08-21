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
	"time"

	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/cmd/data-service/db-migration/migrator"
)

func init() {
	// add current migration to migrator
	migrator.GetMigrator().AddMigration(&migrator.Migration{
		Version: "20260821170000",
		Name:    "20260821170000_realign_client_tenant_id",
		Mode:    migrator.GormMode,
		Up:      mig20260821170000Up,
		Down:    mig20260821170000Down,
	})
}

// 修正 clients / client_events 的 tenant_id。
//
// 多租户环境下，客户端上报链路曾把租户解析成 default 后落库（feed-server 入队时不带租户，
// cache-service 消费时按 biz_id 反查，缓存陈旧或两侧租户开关不一致就会退化成 default），
// 与服务实际所属租户不一致，导致控制台按租户过滤查不到任何客户端。
// 这里以 applications.tenant_id 为准回填——同一个 app 下的客户端必然属于该 app 的租户。
//
// 运维注意：本迁移只修数据。biz_id → tenant_id 的映射还缓存在 Redis
// （key: {bizID}bscp:tenant-id:tenant-id）以及 feed-server / cache-service 的进程内 gcache 中，
// 迁移后需清理该 Redis key 并重启这两个服务，否则新写入的数据仍可能带着旧租户。
func mig20260821170000Up(tx *gorm.DB) error {
	// 回填可能涉及大量行，走独立会话分批提交，避免长时间占用迁移事务的连接
	standaloneDB, err := getStandaloneDB(tx)
	if err != nil {
		return fmt.Errorf("get standalone db failed: %w", err)
	}

	for _, tblName := range []string{"clients", "client_events"} {
		if err := realignTenantWithApp(standaloneDB, tblName); err != nil {
			return fmt.Errorf("realign tenant_id for %s failed: %w", tblName, err)
		}
	}

	return nil
}

// mig20260821170000Down for down migration
// 回填是把错误的 tenant_id 纠正为正确值，原值已无保留意义，回滚不做处理。
func mig20260821170000Down(tx *gorm.DB) error {
	return nil
}

// realignTenantWithApp 按 id 游标分批，把表中与所属 app 不一致的 tenant_id 纠正过来。
// SELECT 与 UPDATE 使用同一套过滤条件，改完的行不再命中，游标严格递增可保证终止。
func realignTenantWithApp(db *gorm.DB, tblName string) error {
	if !tableIdentPattern.MatchString(tblName) {
		return fmt.Errorf("invalid table name %q", tblName)
	}

	// app 自身租户缺失时无法判定正确值，跳过而不是猜成 default
	mismatchCond := `a.tenant_id IS NOT NULL AND a.tenant_id <> ''
		AND (t.tenant_id IS NULL OR t.tenant_id <> a.tenant_id)`

	selSQL := fmt.Sprintf(
		"SELECT t.id FROM %s t INNER JOIN applications a ON a.id = t.app_id WHERE t.id > ? AND %s ORDER BY t.id LIMIT %d",
		quoteTable(tblName), mismatchCond, backfillBatchSize)
	updateSQL := fmt.Sprintf(
		"UPDATE %s t INNER JOIN applications a ON a.id = t.app_id SET t.tenant_id = a.tenant_id WHERE t.id IN (?) AND %s",
		quoteTable(tblName), mismatchCond)

	start := time.Now()
	var lastID uint64
	batchCount, processed := 0, 0

	for {
		var ids []batchIDs
		if err := db.Raw(selSQL, lastID).Scan(&ids).Error; err != nil {
			return fmt.Errorf("select batch ids failed: %w", err)
		}
		if len(ids) == 0 {
			fmt.Printf("realign tenant_id %s done: %d rows in %d batches, cost %s\n",
				tblName, processed, batchCount, time.Since(start).Round(time.Millisecond))
			return nil
		}

		idList := make([]uint64, len(ids))
		for i, r := range ids {
			idList[i] = r.ID
		}
		maxIDInBatch := idList[len(idList)-1]

		if err := execWithRetry(db, updateSQL, idList); err != nil {
			return fmt.Errorf("update batch (%d-%d] failed: %w", lastID, maxIDInBatch, err)
		}

		lastID = maxIDInBatch
		batchCount++
		processed += len(idList)
		if batchCount%progressLogInterval == 0 {
			fmt.Printf("realign tenant_id %s progress: cursor=%d, processed=%d rows, cost %s\n",
				tblName, lastID, processed, time.Since(start).Round(time.Millisecond))
		}
	}
}
