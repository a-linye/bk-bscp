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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/task/executor/common"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

const (
	// defaultMaxBackups 是默认的脚本备份文件最大数量，超过后会删除最旧的备份文件
	defaultMaxBackups = 5
	// GSE 脚本执行账号
	linuxExecutionUser   = "root"
	windowsExecutionUser = "Administrator"
)

// ScriptBuilder 根据 FileMode (OS 类型) 构建不同平台的脚本
type ScriptBuilder struct {
	FileMode   table.FileMode
	MaxBackups int
}

// IsWindows 判断目标平台是否为 Windows
func (b *ScriptBuilder) IsWindows() bool {
	return b.FileMode == table.Windows
}

// BuildConfigPushScript 构建配置下发脚本
func (b *ScriptBuilder) BuildConfigPushScript(base64Content, absPath, fileMode, owner, group string) (string, error) {
	if b.MaxBackups <= 0 {
		b.MaxBackups = defaultMaxBackups
	}
	if b.IsWindows() {
		return b.buildWindowsPushScript(base64Content, absPath, owner, group, b.MaxBackups)
	}
	return buildLinuxPushScript(base64Content, absPath, fileMode, owner, group, b.MaxBackups)
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
func buildLinuxPushScript(base64Content, absPath, fileMode, owner, group string, maxBackups int) (string, error) {
	if !strings.HasPrefix(absPath, "/") {
		return "", fmt.Errorf("absPath must be absolute")
	}

	if !fileModeRe.MatchString(fileMode) {
		return "", fmt.Errorf("invalid fileMode: %s", fileMode)
	}

	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

TARGET_PATH=%s
MAX_BACKUPS=%d

# 1. 目标为软链接时先解析真实路径，保持跟随语义：备份、临时文件与替换都作用于真实路径
if [ -L "$TARGET_PATH" ]; then
    REAL_PATH="$(readlink -f -- "$TARGET_PATH")"
    echo "Resolved symlink: $TARGET_PATH -> $REAL_PATH"
    TARGET_PATH="$REAL_PATH"
fi

TARGET_DIR="$(dirname "$TARGET_PATH")"
TARGET_NAME="$(basename "$TARGET_PATH")"

# 2. 创建目标目录
mkdir -p -- "$TARGET_DIR"

# 3. 临时文件与目标同目录，保证后续 mv 在同一文件系统内落到 rename(2) 从而原子；
#    任一步骤失败都清理临时文件，不留残留、不触碰目标文件
TMP_PATH="${TARGET_PATH}.tmp.$$.${RANDOM}"
trap 'rm -f -- "$TMP_PATH"' EXIT

# 4. 备份原文件（如果存在），并让备份归属与目标文件保持一致
if [ -f "$TARGET_PATH" ]; then
    TIMESTAMP="$(date +%%s)"
    BACKUP_PATH="${TARGET_DIR}/${TARGET_NAME}.${TIMESTAMP}.bak"
    cp -- "$TARGET_PATH" "$BACKUP_PATH"
    chown %s:%s -- "$BACKUP_PATH"
    echo "Backup created: $BACKUP_PATH"

    # 5. 清理旧备份：超过 MAX_BACKUPS 份则删除最旧的
    # 按修改时间从旧到新排列，找出需要删除的文件
    BACKUP_COUNT="$(ls -1 "${TARGET_DIR}/${TARGET_NAME}".*.bak 2>/dev/null | wc -l)"
    if [ "$BACKUP_COUNT" -gt "$MAX_BACKUPS" ]; then
        DELETE_COUNT=$(( BACKUP_COUNT - MAX_BACKUPS ))
        # ls -1t 按时间降序（最新在前），tail 取最旧的
        ls -1t "${TARGET_DIR}/${TARGET_NAME}".*.bak 2>/dev/null \
            | tail -n "$DELETE_COUNT" \
            | xargs -r rm -f --
        echo "Cleaned $DELETE_COUNT old backup(s), kept latest $MAX_BACKUPS"
    fi
fi

# 6. 写入临时文件（base64 解码）
echo %s | base64 -d > "$TMP_PATH"

# 7. 在临时文件上设置权限和属主：失败即终止，目标文件保持原状
chmod %s -- "$TMP_PATH"
chown %s:%s -- "$TMP_PATH"

# 8. 继承目标文件原有的 SELinux 标签。替换换了 inode，新文件默认只能拿到目录的默认标签，
#    原文件若被 chcon 定制过就会退化，导致业务进程被拒绝读取（下发成功但配置加载失败）。
#    SELinux 关闭或未装工具的机器上整段跳过，不影响下发。
if [ -e "$TARGET_PATH" ] && command -v chcon >/dev/null 2>&1; then
    chcon --reference="$TARGET_PATH" -- "$TMP_PATH" 2>/dev/null || true
fi

# 9. 原子替换目标文件
mv -f -- "$TMP_PATH" "$TARGET_PATH"

# 10. 校验（不影响主流程）
set +e
ls -l "$TARGET_PATH" || true
md5sum "$TARGET_PATH" || true
`,
		shellQuote(absPath),
		maxBackups,
		shellQuote(owner),
		shellQuote(group),
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

// newTempToken 生成临时文件名后缀。
// 唯一性必须由服务端保证，不能交给 cmd 的 %RANDOM%：它在 cmd 进程启动时以系统时钟播种，
// 同一时刻被 GSE 拉起的多个脚本会取到完全相同的序列，临时文件因而互相覆盖、互相删除。
func newTempToken() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

// buildWindowsPushScript 构建 Windows 配置下发脚本
//
// base64 中转文件与解码产物都放在目标同目录，而不是曾经的 %TEMP%：
// 一是 %TEMP% 全机共享，同一台主机上多个任务下发同名配置（若干个 .NET 服务各有一份
// Web.config，只是目录不同）会撞进同一个命名空间；
// 二是 %TEMP% 与目标常常不在同一个卷上，move 会退化成「拷贝 + 删除」，
// 业务进程有机会读到只写了一半的配置，放同目录后才能走同卷内的原子 rename。
func (b *ScriptBuilder) buildWindowsPushScript(base64Content, absPath, owner, group string, maxBackups int) (string, error) {
	winPath := ToWindowsPath(absPath)
	token := newTempToken()

	return fmt.Sprintf(`@echo off
setlocal enabledelayedexpansion

set "TARGET_PATH=%s"
set /a MAX_BACKUPS=%d

REM 1. 解析目录和文件名
for %%%%i in ("%%TARGET_PATH%%") do (
    set "TARGET_DIR=%%%%~dpi"
    set "TARGET_NAME=%%%%~nxi"
)

REM 2. 创建目标目录。临时文件也落在这里，建不出来就必须立刻失败，
REM    否则错误会一路滑到后面的 move，报成含义完全不同的「找不到文件」。
if not exist "!TARGET_DIR!" mkdir "!TARGET_DIR!"
if not exist "!TARGET_DIR!" (
    echo MKDIR_FAILED
    exit /b 1
)

if exist "!TARGET_PATH!" (
    echo [INFO] 发现原文件，准备备份...

    REM 获取时间戳
    for /f "delims=" %%%%i in (
        'powershell -NoProfile -Command "Get-Date -Format yyyyMMddHHmmss"'
    ) do set "STAMP=%%%%i"

    echo [INFO] 时间戳: !STAMP!

    set "BACKUP_FILE=!TARGET_NAME!.!STAMP!.bak"
    set "BACKUP_FULL_PATH=!TARGET_DIR!!BACKUP_FILE!"

    copy /y "!TARGET_PATH!" "!BACKUP_FULL_PATH!" >nul || (
        echo [ERROR] 备份失败
        exit /b 1
    )
    echo [OK] 备份已生成: !BACKUP_FILE!

    REM 3. 统计备份数量
    set /a COUNT=0
    for /f "delims=" %%%%f in (
        'dir /b /o:d "!TARGET_DIR!!TARGET_NAME!.*.bak" 2^>nul'
    ) do set /a COUNT+=1

    echo [INFO] 当前备份数: !COUNT! / 最大保留: %%MAX_BACKUPS%%

    REM 4. 删除最旧备份（无临时文件版本）
    if !COUNT! gtr %%MAX_BACKUPS%% (
        set /a DEL_COUNT=!COUNT!-%%MAX_BACKUPS%%
        echo [INFO] 需删除最旧备份数: !DEL_COUNT!

        set /a IDX=0
        for /f "delims=" %%%%f in (
            'dir /b /o:d "!TARGET_DIR!!TARGET_NAME!.*.bak" 2^>nul'
        ) do (
            if !IDX! lss !DEL_COUNT! (
                echo [CLEAN] 删除旧备份: %%%%f
                del /f /q "!TARGET_DIR!%%%%f" >nul 2>&1
                set /a IDX+=1
            )
        )
    )
) else (
    echo [INFO] 目标文件不存在，跳过备份。
)

REM 5. 写入配置文件（base64 解码）。临时文件与目标同目录，保证后续 move 在同卷内原子完成；
REM    文件名后缀由服务端生成，全局唯一，避免同主机并发任务互相覆盖。
REM    base64 由服务端切成多行追加写入，单行绝不能超过 cmd 的行长上限，详见 windowsB64ChunkSize。
set "BSCP_TMP=!TARGET_DIR!!TARGET_NAME!.bscp.%s.b64"
set "BSCP_OUT=!TARGET_DIR!!TARGET_NAME!.bscp.%s.out"
del /f /q "!BSCP_TMP!" >nul 2>&1
del /f /q "!BSCP_OUT!" >nul 2>&1
%s
if not exist "!BSCP_TMP!" (
    echo WRITE_TMP_FAILED
    exit /b 1
)
REM certutil 默认拒绝覆盖已存在的输出文件，会报 0x80070050 ERROR_FILE_EXISTS，必须带 -f
certutil -f -decode "!BSCP_TMP!" "!BSCP_OUT!"
if !ERRORLEVEL! neq 0 (
    echo DECODE_FAILED
    del /f /q "!BSCP_TMP!" >nul 2>&1
    del /f /q "!BSCP_OUT!" >nul 2>&1
    exit /b 1
)
del /f /q "!BSCP_TMP!" >nul 2>&1
move /y "!BSCP_OUT!" "%%TARGET_PATH%%" >nul || (
    echo MOVE_FAILED
    del /f /q "!BSCP_OUT!" >nul 2>&1
    exit /b 1
)

REM 6. 设置属主与权限。脚本以 Administrator 运行，
REM    /setowner 需要接管所有权的特权，改为该账号执行后才能真正生效。
REM    这里不静默丢弃错误，失败时回传 errorlevel 便于定位，但不终止下发。
icacls "%%TARGET_PATH%%" /setowner "%s" >nul
if !ERRORLEVEL! neq 0 echo [WARN] icacls /setowner failed, errorlevel=!ERRORLEVEL!
icacls "%%TARGET_PATH%%" /grant:r "%s:(F)" >nul
if !ERRORLEVEL! neq 0 echo [WARN] icacls grant owner full control failed, errorlevel=!ERRORLEVEL!
icacls "%%TARGET_PATH%%" /grant:r "%s:(R)" >nul
if !ERRORLEVEL! neq 0 echo [WARN] icacls grant group read failed, errorlevel=!ERRORLEVEL!

REM 7. 校验
dir "%%TARGET_PATH%%"
certutil -hashfile "%%TARGET_PATH%%" MD5

endlocal
`, winPath, maxBackups, token, token, buildWindowsB64WriteLines(base64Content), owner, owner, group), nil
}

// windowsB64ChunkSize 是 base64 中转文件每行写入的字符数。
// cmd.exe 解析批处理时单行有约 8191 字符的上限，超出部分被静默丢弃。整份 base64 写在一行里，
// 配置超过 6 KB 左右就会被截断，而 certutil 仍会把残缺的 base64 解码成一个语法上"看起来正常"
// 的文件，move 和退出码全部正常，下发任务照样报成功——故障只能靠人工比对配置才能发现。
// 必须是 4 的倍数，否则行尾会切在 base64 四字符组中间。
const windowsB64ChunkSize = 1024

// buildWindowsB64WriteLines 把 base64 内容切成多行 echo，逐行追加进中转文件。
//
// 重定向写在 echo 之前而不是行尾，有两个原因：
// 一是 `echo x > f` 会把 `>` 前的那个空格一起写进文件；
// 二是 base64 字母表含数字，`echo ...1>f` 里的末位数字会被 cmd 当作重定向句柄吃掉，
// 导致内容静默少一个字符。base64 字母表（A-Za-z0-9+/=）不含 cmd 元字符，无需转义。
func buildWindowsB64WriteLines(base64Content string) string {
	// 空内容也要把中转文件建出来，否则后面的存在性检查会报成含义不同的 WRITE_TMP_FAILED
	if base64Content == "" {
		return `>>"!BSCP_TMP!" echo.`
	}

	var b strings.Builder
	for i := 0; i < len(base64Content); i += windowsB64ChunkSize {
		end := i + windowsB64ChunkSize
		if end > len(base64Content) {
			end = len(base64Content)
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(`>>"!BSCP_TMP!" echo `)
		b.WriteString(base64Content[i:end])
	}
	return b.String()
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

// ScriptStoreDirByFileMode 根据平台返回脚本存放目录
func ScriptStoreDirByFileMode(linuxBaseDir, windowsScriptDir string,
	fileMode table.FileMode) string {
	if fileMode == table.Windows {
		return windowsScriptDir
	}
	return linuxBaseDir
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

// GetExecutionUser 返回 GSE 脚本的执行账号
// Linux 固定为 root，Windows 固定为 Administrator
func GetExecutionUser(fileMode table.FileMode) string {
	if fileMode == table.Windows {
		return windowsExecutionUser
	}
	return linuxExecutionUser
}

// ToWindowsPath 将 POSIX 路径转换为 Windows 路径
func ToWindowsPath(posixPath string) string {
	return strings.ReplaceAll(posixPath, "/", `\`)
}
