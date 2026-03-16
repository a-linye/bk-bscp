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

package common

import (
	"fmt"
	"io"

	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/repository"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// ResolveGincludeTemplate resolves a Ginclude template reference by name.
// It looks up the config template by name, gets the latest revision, and downloads the content from repo.
func ResolveGincludeTemplate(kt *kit.Kit, daoSet dao.Set, repo repository.Provider,
	bizID uint32, templateName string) (string, error) {

	// 1. 通过名称查找配置模板
	configTemplates, err := daoSet.ConfigTemplate().ListByNames(kt, bizID, []string{templateName})
	if err != nil {
		return "", fmt.Errorf("query config template by name %q failed: %w", templateName, err)
	}
	if len(configTemplates) == 0 {
		return "", fmt.Errorf("config template %q not found", templateName)
	}
	configTemplate := configTemplates[0]

	// 2. 获取关联的 Template 最新版本
	templateID := configTemplate.Attachment.TemplateID
	revision, err := daoSet.TemplateRevision().GetLatestTemplateRevision(kt, bizID, templateID)
	if err != nil {
		return "", fmt.Errorf("get latest revision for template %q (id=%d) failed: %w",
			templateName, templateID, err)
	}

	// 3. 从仓库下载内容
	if revision.Spec == nil || revision.Spec.ContentSpec == nil || revision.Spec.ContentSpec.Signature == "" {
		return "", fmt.Errorf("template %q has no content signature", templateName)
	}

	body, _, err := repo.Download(kt, revision.Spec.ContentSpec.Signature)
	if err != nil {
		return "", fmt.Errorf("download template %q content failed: %w", templateName, err)
	}
	defer body.Close()

	content, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("read template %q content failed: %w", templateName, err)
	}

	return string(content), nil
}
