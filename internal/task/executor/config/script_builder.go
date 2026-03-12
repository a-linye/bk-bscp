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

package config

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/task/executor/common"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

// ScriptBuilder 根据 FileMode (OS 类型) 构建不同平台的脚本
type ScriptBuilder struct {
	FileMode table.FileMode
}

// IsWindows 判断目标平台是否为 Windows
func (b *ScriptBuilder) IsWindows() bool {
	return b.FileMode == table.Windows
}

// BuildConfigPushScript 构建配置下发脚本
func (b *ScriptBuilder) BuildConfigPushScript(base64Content, absPath, fileMode, owner, group string) (string, error) {
	if b.IsWindows() {
		return b.buildWindowsPushScript(base64Content, absPath)
	}
	return buildLinuxPushScript(base64Content, absPath, fileMode, owner, group)
}

// BuildFileMD5Script 构建计算文件 MD5 的脚本
func (b *ScriptBuilder) BuildFileMD5Script(absPath string) (string, error) {
	if b.IsWindows() {
		return b.buildWindowsMD5Script(absPath)
	}
	return buildLinuxMD5Script(absPath)
}

// BuildFileCatScript 构建读取文件内容的脚本
func (b *ScriptBuilder) BuildFileCatScript(absPath string) (string, error) {
	if b.IsWindows() {
		return b.buildWindowsCatScript(absPath)
	}
	return buildLinuxCatScript(absPath)
}

// ---- Linux 脚本 ----

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\'"\'"'`) + "'"
}

var fileModeRe = regexp.MustCompile(`^[0-7]{3,4}$`)

// buildLinuxPushScript 构建 Linux 配置下发脚本
func buildLinuxPushScript(base64Content, absPath, fileMode, owner, group string) (string, error) {
	if !strings.HasPrefix(absPath, "/") {
		return "", fmt.Errorf("absPath must be absolute")
	}

	if !fileModeRe.MatchString(fileMode) {
		return "", fmt.Errorf("invalid fileMode: %s", fileMode)
	}

	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

TARGET_PATH=%s
TARGET_DIR="$(dirname "$TARGET_PATH")"

# 1. 创建目标目录
mkdir -p -- "$TARGET_DIR"

# 2. 写入配置文件（base64 解码）
echo %s | base64 -d > "$TARGET_PATH"

# 3. 设置权限和属主
chmod %s "$TARGET_PATH"
chown %s:%s "$TARGET_PATH"

# 4. 校验（不影响主流程）
set +e
ls -l "$TARGET_PATH" || true
md5sum "$TARGET_PATH" || true
`,
		shellQuote(absPath),
		shellQuote(base64Content),
		fileMode,
		shellQuote(owner),
		shellQuote(group),
	), nil
}

// buildLinuxMD5Script 构建 Linux MD5 校验脚本
func buildLinuxMD5Script(absPath string) (string, error) {
	if !strings.HasPrefix(absPath, "/") {
		return "", fmt.Errorf("absPath must be absolute")
	}

	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

TARGET_PATH=%s

md5sum "$TARGET_PATH" | awk '{print $1}'
`,
		shellQuote(absPath),
	), nil
}

// buildLinuxCatScript 构建 Linux 文件内容读取脚本
func buildLinuxCatScript(absPath string) (string, error) {
	if !strings.HasPrefix(absPath, "/") {
		return "", fmt.Errorf("absPath must be absolute")
	}

	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

TARGET_PATH=%s

cat "$TARGET_PATH"
`,
		shellQuote(absPath),
	), nil
}

// ---- Windows 脚本 ----

// buildWindowsPushScript 构建 Windows 配置下发脚本
func (b *ScriptBuilder) buildWindowsPushScript(base64Content, absPath string) (string, error) {
	winPath := ToWindowsPath(absPath)

	return fmt.Sprintf(`@echo off
setlocal enabledelayedexpansion

set "TARGET_PATH=%s"

REM 1. 创建目标目录
for %%%%i in ("%%TARGET_PATH%%") do set "TARGET_DIR=%%%%~dpi"
if not exist "%%TARGET_DIR%%" mkdir "%%TARGET_DIR%%"

REM 2. 写入配置文件（base64 解码）
echo %s > "%%TEMP%%\bscp_tmp.b64"
certutil -decode "%%TEMP%%\bscp_tmp.b64" "%%TARGET_PATH%%" >nul
del "%%TEMP%%\bscp_tmp.b64"

REM 3. 校验
dir "%%TARGET_PATH%%"
certutil -hashfile "%%TARGET_PATH%%" MD5

endlocal
`, winPath, base64Content), nil
}

// buildWindowsMD5Script 构建 Windows MD5 校验脚本
func (b *ScriptBuilder) buildWindowsMD5Script(absPath string) (string, error) {
	winPath := ToWindowsPath(absPath)

	return fmt.Sprintf(`@echo off
for /f "skip=1 tokens=*" %%%%a in ('certutil -hashfile "%s" MD5 ^| findstr /v "CertUtil"') do (
    echo %%%%a
    goto :eof
)
`, winPath), nil
}

// buildWindowsCatScript 构建 Windows 文件内容读取脚本
func (b *ScriptBuilder) buildWindowsCatScript(absPath string) (string, error) {
	winPath := ToWindowsPath(absPath)

	return fmt.Sprintf(`@echo off
if exist "%s" (
    type "%s"
) else (
    echo FILE_NOT_FOUND
    exit /b 1
)
`, winPath, winPath), nil
}

// ---- 公共辅助函数 ----

// scriptStoreDir 返回 Linux 脚本存放目录: {baseDir}/{agentUser}/
func scriptStoreDir(baseDir, agentUser string) string {
	return path.Join(baseDir, agentUser)
}

// ScriptStoreDirByFileMode 根据平台返回脚本存放目录
func ScriptStoreDirByFileMode(linuxBaseDir, linuxAgentUser, windowsScriptDir string,
	fileMode table.FileMode) string {
	if fileMode == table.Windows {
		return windowsScriptDir
	}
	return scriptStoreDir(linuxBaseDir, linuxAgentUser)
}

// BuildScriptNameByFileMode 生成脚本文件名（区分平台后缀）
func BuildScriptNameByFileMode(action string, p *common.TaskPayload, fileMode table.FileMode) string {
	ext := ".sh"
	if fileMode == table.Windows {
		ext = ".bat"
	}
	return fmt.Sprintf("bk_gse_script_%s_%d_%d_%d_%d%s",
		action,
		time.Now().Unix(),
		p.ConfigPayload.ConfigTemplateID,
		p.ProcessPayload.CcProcessID,
		p.ProcessPayload.ModuleInstSeq,
		ext,
	)
}

// BuildScriptCommand 根据平台构建脚本执行命令
func BuildScriptCommand(storeDir, scriptName string, fileMode table.FileMode) string {
	if fileMode == table.Windows {
		return ToWindowsPath(storeDir) + `\` + scriptName
	}
	return path.Join(storeDir, scriptName)
}

// GetExecutionUser 根据平台返回执行账号
func GetExecutionUser(fileMode table.FileMode, configUser string) string {
	if fileMode == table.Windows {
		if configUser == "" {
			return "Administrator"
		}
		return configUser
	}
	if configUser == "" {
		return "root"
	}
	return configUser
}

// ToWindowsPath 将 POSIX 路径转换为 Windows 路径
func ToWindowsPath(posixPath string) string {
	return strings.ReplaceAll(posixPath, "/", `\`)
}
