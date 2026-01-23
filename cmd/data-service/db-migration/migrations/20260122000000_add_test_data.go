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

// Package migrations 用于数据迁移测试的测试数据
package migrations

import (
	"time"

	"gorm.io/gorm"

	"github.com/TencentBlueKing/bk-bscp/cmd/data-service/db-migration/migrator"
)

func init() {
	migrator.GetMigrator().AddMigration(&migrator.Migration{
		Version: "20260122000000",
		Name:    "20260122000000_add_test_data",
		Mode:    migrator.GormMode,
		Up:      mig20260122000000Up,
		Down:    mig20260122000000Down,
	})
}

// 测试数据使用的常量
const (
	testShardingDBID    = 1
	testBizID           = 100
	testAppID           = 1001
	testReleaseID       = 2001
	testConfigItemID    = 3001
	testCommitID        = 4001
	testContentID       = 5001
	testStrategySetID   = 6001
	testStrategyID      = 7001
	testGroupID         = 8001
	testHookID          = 9001
	testHookRevisionID  = 9101
	testCredentialID    = 10001
	testCredScopeID     = 10101
	testTemplateSpaceID = 11001
	testTemplateID      = 12001
	testTemplateRevID   = 13001
	testTemplateSetID   = 14001
	testTemplateVarID   = 15001
	testAppTplBindID    = 16001
	testAppTplVarID     = 17001
	testRelAppTplID     = 18001
	testRelAppTplVarID  = 19001
	testKvID            = 20001
	testReleasedKvID    = 21001
	testReleasedCIID    = 22001
	testReleasedHookID  = 23001
	testReleasedGroupID = 24001
	testCPSID           = 25001 // current_published_strategies
	testGroupAppBindID  = 26001
)

// mig20260122000000Up 插入测试数据
func mig20260122000000Up(tx *gorm.DB) error {
	now := time.Now()
	creator := "test_user"

	// ==================== 第一批：基础表 ====================

	// 0. sharding_dbs - 分片数据库配置（sharding_bizs 的外键依赖）
	if err := tx.Exec(`INSERT INTO sharding_dbs (id, type, host, port, user, password, `+"`database`"+`, memo, creator, reviser, created_at, updated_at) 
		VALUES (?, 'mysql', 'localhost', 3306, 'root', 'test_pwd', 'bk_bscp', 'test sharding db', ?, ?, ?, ?)`,
		testShardingDBID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 1. sharding_bizs - 业务分片配置
	if err := tx.Exec(`INSERT INTO sharding_bizs (id, memo, biz_id, sharding_db_id, creator, reviser, created_at, updated_at) 
		VALUES (1, 'test biz sharding', ?, ?, ?, ?, ?, ?)`,
		testBizID, testShardingDBID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 2. applications - 应用定义
	// 字段变更：mode, reload_type, reload_file_path 已删除；新增 alias, data_type
	if err := tx.Exec(`INSERT INTO applications (id, name, config_type, memo, alias, data_type, biz_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'test-app-001', 'file', 'test application', 'test-app-alias', '', ?, ?, ?, ?, ?)`,
		testAppID, testBizID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 3. template_spaces - 模板空间
	if err := tx.Exec(`INSERT INTO template_spaces (id, name, memo, biz_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'test-template-space', 'test template space', ?, ?, ?, ?, ?)`,
		testTemplateSpaceID, testBizID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 4. groups - 分组
	if err := tx.Exec(`INSERT INTO `+"`groups`"+` (id, name, mode, public, selector, uid, biz_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'test-group', 'custom', true, '{"labels_and":[{"key":"env","op":"eq","value":"test"}]}', 'group-uid-001', ?, ?, ?, ?, ?)`,
		testGroupID, testBizID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 5. hooks - 钩子脚本
	// 字段变更：tag 已替换为 tags (JSON)
	if err := tx.Exec(`INSERT INTO hooks (id, name, memo, type, tags, biz_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'test-hook', 'test hook script', 'shell', '["pre"]', ?, ?, ?, ?, ?)`,
		testHookID, testBizID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 6. credentials - 凭证
	// 字段变更：新增 name 字段
	if err := tx.Exec(`INSERT INTO credentials (id, name, biz_id, credential_type, enc_credential, enc_algorithm, memo, enable, creator, reviser, created_at, updated_at, expired_at) 
		VALUES (?, 'test-credential', ?, 'bearer', 'encrypted_token_test', 'aes', 'test credential', 1, ?, ?, ?, ?, ?)`,
		testCredentialID, testBizID, creator, creator, now, now, now.Add(365*24*time.Hour)).Error; err != nil {
		return err
	}

	// ==================== 第二批：二级表 ====================

	// 7. config_items - 配置项
	// 字段变更：新增 charset 字段
	if err := tx.Exec(`INSERT INTO config_items (id, name, path, file_type, file_mode, memo, user, user_group, privilege, charset, biz_id, app_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'config.yaml', '/etc/app', 'yaml', 'unix', 'test config item', 'root', 'root', '644', '', ?, ?, ?, ?, ?, ?)`,
		testConfigItemID, testBizID, testAppID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 8. releases - 发布版本
	// 字段变更：新增 fully_released 字段
	if err := tx.Exec(`INSERT INTO releases (id, name, memo, deprecated, publish_num, fully_released, biz_id, app_id, creator, created_at) 
		VALUES (?, 'v1.0.0', 'first release', false, 1, true, ?, ?, ?, ?)`,
		testReleaseID, testBizID, testAppID, creator, now).Error; err != nil {
		return err
	}

	// 9. strategy_sets - 策略集
	if err := tx.Exec(`INSERT INTO strategy_sets (id, name, mode, status, memo, biz_id, app_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'default-strategy-set', 'normal', 'enabled', 'default strategy set', ?, ?, ?, ?, ?, ?)`,
		testStrategySetID, testBizID, testAppID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 10. template_sets - 模板套餐
	if err := tx.Exec(`INSERT INTO template_sets (id, name, memo, template_ids, public, bound_apps, biz_id, template_space_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'test-template-set', 'test template set', '[]', true, '[]', ?, ?, ?, ?, ?, ?)`,
		testTemplateSetID, testBizID, testTemplateSpaceID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 11. templates - 配置模板
	if err := tx.Exec(`INSERT INTO templates (id, name, path, memo, biz_id, template_space_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'nginx.conf', '/etc/nginx', 'nginx config template', ?, ?, ?, ?, ?, ?)`,
		testTemplateID, testBizID, testTemplateSpaceID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 12. template_variables - 模板变量
	if err := tx.Exec(`INSERT INTO template_variables (id, name, type, default_val, memo, biz_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'SERVER_PORT', 'string', '8080', 'server port variable', ?, ?, ?, ?, ?)`,
		testTemplateVarID, testBizID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 13. hook_revisions - 钩子版本
	if err := tx.Exec(`INSERT INTO hook_revisions (id, name, memo, state, content, biz_id, hook_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'v1.0.0', 'first version', 'not_deployed', '#!/bin/bash\necho "pre hook"', ?, ?, ?, ?, ?, ?)`,
		testHookRevisionID, testBizID, testHookID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 14. credential_scopes - 凭证作用域
	if err := tx.Exec(`INSERT INTO credential_scopes (id, biz_id, credential_id, credential_scope, creator, reviser, updated_at, created_at, expired_at) 
		VALUES (?, ?, ?, 'app:test-app-001', ?, ?, ?, ?, ?)`,
		testCredScopeID, testBizID, testCredentialID, creator, creator, now, now, now.Add(365*24*time.Hour)).Error; err != nil {
		return err
	}

	// 15. group_app_binds - 分组应用绑定
	if err := tx.Exec(`INSERT INTO group_app_binds (id, group_id, app_id, biz_id) 
		VALUES (?, ?, ?, ?)`,
		testGroupAppBindID, testGroupID, testAppID, testBizID).Error; err != nil {
		return err
	}

	// ==================== 第三批：依赖表 ====================

	// 16. contents - 内容记录
	// 字段变更：新增 md5 字段
	if err := tx.Exec(`INSERT INTO contents (id, signature, byte_size, md5, biz_id, app_id, config_item_id, creator, created_at) 
		VALUES (?, 'sha256_signature_test_001', 1024, 'md5_test_001', ?, ?, ?, ?, ?)`,
		testContentID, testBizID, testAppID, testConfigItemID, creator, now).Error; err != nil {
		return err
	}

	// 17. commits - 提交记录
	// 字段变更：新增 md5 字段
	if err := tx.Exec(`INSERT INTO commits (id, content_id, signature, byte_size, md5, memo, biz_id, app_id, config_item_id, creator, created_at) 
		VALUES (?, ?, 'sha256_signature_test_001', 1024, 'md5_test_001', 'initial commit', ?, ?, ?, ?, ?)`,
		testCommitID, testContentID, testBizID, testAppID, testConfigItemID, creator, now).Error; err != nil {
		return err
	}

	// 18. strategies - 策略
	// 字段变更：mode 已删除；新增多个审批相关字段
	if err := tx.Exec(`INSERT INTO strategies (id, name, release_id, as_default, scope, namespace, memo, pub_state, biz_id, app_id, strategy_set_id, itsm_ticket_state_id,creator, reviser, created_at, updated_at) 
		VALUES (?, 'default-strategy', ?, true, null, '', 'default publish strategy', 'published', ?, ?, ?, ?, ?, ?, ?, ?)`,
		testStrategyID, testReleaseID, testBizID, testAppID, testStrategySetID, 1, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 19. current_published_strategies - 当前发布的策略
	// 注意：current_published_strategies 表仍保留 mode 字段（只有 strategies 表删除了）
	if err := tx.Exec(`INSERT INTO current_published_strategies (id, name, release_id, as_default, scope, mode, namespace, memo, pub_state, biz_id, app_id, strategy_set_id, strategy_id, creator, created_at) 
		VALUES (?, 'default-strategy', ?, true, null, 'normal', '', 'current published', 'published', ?, ?, ?, ?, ?, ?)`,
		testCPSID, testReleaseID, testBizID, testAppID, testStrategySetID, testStrategyID, creator, now).Error; err != nil {
		return err
	}

	// 20. kvs - KV配置
	// 字段变更：新增 signature, md5, byte_size, memo 字段
	if err := tx.Exec(`INSERT INTO kvs (id, `+"`key`"+`, version, kv_type, kv_state, signature, md5, byte_size, memo, biz_id, app_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'database_url', 1, 'string', 'add', 'kv_sha256_001', 'kv_md5_001', 64, 'test kv', ?, ?, ?, ?, ?, ?)`,
		testKvID, testBizID, testAppID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 21. released_config_items - 已发布配置项
	// 字段变更：新增 md5, charset, origin_signature, origin_byte_size 字段
	if err := tx.Exec(`INSERT INTO released_config_items (id, commit_id, release_id, biz_id, app_id, config_item_id, content_id, signature, byte_size, md5, origin_signature, origin_byte_size, name, path, file_type, file_mode, charset, memo, user, user_group, privilege, creator, reviser, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, 'sha256_signature_test_001', 1024, 'md5_test_001', 'sha256_origin_001', 1024, 'config.yaml', '/etc/app', 'yaml', 'unix', '', 'released config', 'root', 'root', '644', ?, ?, ?, ?)`,
		testReleasedCIID, testCommitID, testReleaseID, testBizID, testAppID, testConfigItemID, testContentID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 22. released_groups - 已发布的分组
	if err := tx.Exec(`INSERT INTO released_groups (id, group_id, app_id, release_id, strategy_id, mode, selector, uid, edited, biz_id, reviser, updated_at) 
		VALUES (?, ?, ?, ?, ?, 'custom', '{"labels_and":[{"key":"env","op":"eq","value":"test"}]}', 'group-uid-001', false, ?, ?, ?)`,
		testReleasedGroupID, testGroupID, testAppID, testReleaseID, testStrategyID, testBizID, creator, now).Error; err != nil {
		return err
	}

	// 23. released_hooks - 已发布的钩子
	if err := tx.Exec(`INSERT INTO released_hooks (id, app_id, release_id, hook_type, hook_id, hook_revision_id, hook_name, hook_revision_name, content, script_type, biz_id, reviser, updated_at) 
		VALUES (?, ?, ?, 'pre', ?, ?, 'test-hook', 'v1.0.0', '#!/bin/bash\necho "pre hook"', 'shell', ?, ?, ?)`,
		testReleasedHookID, testAppID, testReleaseID, testHookID, testHookRevisionID, testBizID, creator, now).Error; err != nil {
		return err
	}

	// 24. released_kvs - 已发布KV
	// 字段变更：新增 signature, md5, byte_size, memo 字段
	if err := tx.Exec(`INSERT INTO released_kvs (id, `+"`key`"+`, version, release_id, kv_type, signature, md5, byte_size, memo, biz_id, app_id, creator, reviser, created_at, updated_at) 
		VALUES (?, 'database_url', 1, ?, 'string', 'rkv_sha256_001', 'rkv_md5_001', 64, 'released kv', ?, ?, ?, ?, ?, ?)`,
		testReleasedKvID, testReleaseID, testBizID, testAppID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 25. template_revisions - 模板版本
	// 字段变更：新增 md5, charset 字段
	if err := tx.Exec(`INSERT INTO template_revisions (id, revision_name, revision_memo, name, path, file_type, file_mode, charset, user, user_group, privilege, signature, byte_size, md5, biz_id, template_space_id, template_id, creator, created_at) 
		VALUES (?, 'v1.0.0', 'first revision', 'nginx.conf', '/etc/nginx', 'text', 'unix', '', 'root', 'root', '644', 'sha256_tpl_sign_001', 2048, 'tpl_md5_001', ?, ?, ?, ?, ?)`,
		testTemplateRevID, testBizID, testTemplateSpaceID, testTemplateID, creator, now).Error; err != nil {
		return err
	}

	// 26. app_template_bindings - 应用模板绑定
	if err := tx.Exec(`INSERT INTO app_template_bindings (id, template_space_ids, template_set_ids, template_ids, template_revision_ids, latest_template_ids, bindings, biz_id, app_id, creator, reviser, created_at, updated_at) 
		VALUES (?, '[]', '[]', '[]', '[]', '[]', '[]', ?, ?, ?, ?, ?, ?)`,
		testAppTplBindID, testBizID, testAppID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 27. app_template_variables - 应用模板变量
	if err := tx.Exec(`INSERT INTO app_template_variables (id, variables, biz_id, app_id, creator, reviser, created_at, updated_at) 
		VALUES (?, '[]', ?, ?, ?, ?, ?, ?)`,
		testAppTplVarID, testBizID, testAppID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 28. released_app_templates - 已发布应用模板
	// 字段变更：新增 md5, charset 字段
	if err := tx.Exec(`INSERT INTO released_app_templates (id, release_id, template_space_id, template_space_name, template_set_id, template_set_name, template_id, name, path, template_revision_id, is_latest, template_revision_name, template_revision_memo, file_type, file_mode, charset, user, user_group, privilege, signature, byte_size, md5, origin_signature, origin_byte_size, biz_id, app_id, creator, reviser, created_at, updated_at) 
		VALUES (?, ?, ?, 'test-template-space', ?, 'test-template-set', ?, 'nginx.conf', '/etc/nginx', ?, true, 'v1.0.0', 'released revision', 'text', 'unix', '', 'root', 'root', '644', 'sha256_rel_tpl_001', 2048, 'rel_tpl_md5_001', 'sha256_origin_001', 2048, ?, ?, ?, ?, ?, ?)`,
		testRelAppTplID, testReleaseID, testTemplateSpaceID, testTemplateSetID, testTemplateID, testTemplateRevID, testBizID, testAppID, creator, creator, now, now).Error; err != nil {
		return err
	}

	// 29. released_app_template_variables - 已发布应用模板变量
	if err := tx.Exec(`INSERT INTO released_app_template_variables (id, release_id, variables, biz_id, app_id, creator, created_at) 
		VALUES (?, ?, '[]', ?, ?, ?, ?)`,
		testRelAppTplVarID, testReleaseID, testBizID, testAppID, creator, now).Error; err != nil {
		return err
	}

	// ==================== 更新 id_generators ====================
	idGeneratorUpdates := map[string]uint{
		"applications":                    testAppID,
		"config_items":                    testConfigItemID,
		"commits":                         testCommitID,
		"contents":                        testContentID,
		"releases":                        testReleaseID,
		"released_config_items":           testReleasedCIID,
		"strategies":                      testStrategyID,
		"strategy_sets":                   testStrategySetID,
		"current_published_strategies":    testCPSID,
		"groups":                          testGroupID,
		"group_app_binds":                 testGroupAppBindID,
		"released_groups":                 testReleasedGroupID,
		"hooks":                           testHookID,
		"hook_revisions":                  testHookRevisionID,
		"released_hooks":                  testReleasedHookID,
		"credentials":                     testCredentialID,
		"credential_scopes":               testCredScopeID,
		"template_spaces":                 testTemplateSpaceID,
		"templates":                       testTemplateID,
		"template_revisions":              testTemplateRevID,
		"template_sets":                   testTemplateSetID,
		"template_variables":              testTemplateVarID,
		"app_template_bindings":           testAppTplBindID,
		"app_template_variables":          testAppTplVarID,
		"released_app_templates":          testRelAppTplID,
		"released_app_template_variables": testRelAppTplVarID,
		"kvs":                             testKvID,
		"released_kvs":                    testReleasedKvID,
	}

	for resource, maxID := range idGeneratorUpdates {
		if err := tx.Exec(`UPDATE id_generators SET max_id = ? WHERE resource = ?`, maxID, resource).Error; err != nil {
			return err
		}
	}

	return nil
}

// mig20260122000000Down 删除测试数据
func mig20260122000000Down(tx *gorm.DB) error {
	// 按依赖的逆序删除数据

	// 第三批（依赖表）
	tables := []string{
		"released_app_template_variables",
		"released_app_templates",
		"app_template_variables",
		"app_template_bindings",
		"template_revisions",
		"released_kvs",
		"released_hooks",
		"released_groups",
		"released_config_items",
		"kvs",
		"current_published_strategies",
		"strategies",
		"commits",
		"contents",
		"group_app_binds",
		"credential_scopes",
		"hook_revisions",
		"template_variables",
		"templates",
		"template_sets",
		"strategy_sets",
		"releases",
		"config_items",
		"credentials",
		"hooks",
		"`groups`",
		"template_spaces",
		"applications",
		"sharding_bizs",
	}

	for _, table := range tables {
		if err := tx.Exec("DELETE FROM "+table+" WHERE biz_id = ?", testBizID).Error; err != nil {
			return err
		}
	}

	// 删除 sharding_dbs（该表没有 biz_id 字段，按 id 删除）
	if err := tx.Exec("DELETE FROM sharding_dbs WHERE id = ?", testShardingDBID).Error; err != nil {
		return err
	}

	// 重置 id_generators
	resources := []string{
		"applications", "config_items", "commits", "contents", "releases",
		"released_config_items", "strategies", "strategy_sets", "current_published_strategies",
		"groups", "group_app_binds", "released_groups", "hooks", "hook_revisions",
		"released_hooks", "credentials", "credential_scopes", "template_spaces",
		"templates", "template_revisions", "template_sets", "template_variables",
		"app_template_bindings", "app_template_variables", "released_app_templates",
		"released_app_template_variables", "kvs", "released_kvs",
	}

	for _, resource := range resources {
		if err := tx.Exec(`UPDATE id_generators SET max_id = 0 WHERE resource = ?`, resource).Error; err != nil {
			return err
		}
	}

	return nil
}
