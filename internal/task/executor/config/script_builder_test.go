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
	"encoding/base64"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
)

func TestGetExecutionUser(t *testing.T) {
	tests := []struct {
		name     string
		fileMode table.FileMode
		want     string
	}{
		{"linux uses root", table.Unix, linuxExecutionUser},
		{"windows uses administrator", table.Windows, windowsExecutionUser},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetExecutionUser(tt.fileMode))
		})
	}
}

func TestBuildLinuxPushScriptRejectsInvalidInput(t *testing.T) {
	builder := &ScriptBuilder{FileMode: table.Unix}

	_, err := builder.BuildConfigPushScript("Y29udGVudA==", "relative/path.conf", "644", "root", "root")
	assert.ErrorContains(t, err, "absPath must be absolute")

	_, err = builder.BuildConfigPushScript("Y29udGVudA==", "/etc/app.conf", "888", "root", "root")
	assert.ErrorContains(t, err, "invalid fileMode")
}

// TestBuildLinuxPushScriptAtomicOrder 校验脚本按「解析软链接 -> 备份 -> 写临时文件 -> 设权限属主 -> 原子替换」编排，
// 权限与属主必须设置在临时文件上，否则失败时会留下属主不正确的目标文件。
func TestBuildLinuxPushScriptAtomicOrder(t *testing.T) {
	builder := &ScriptBuilder{FileMode: table.Unix}

	script, err := builder.BuildConfigPushScript("Y29udGVudA==", "/etc/app/app.conf", "444", "www-data", "www-data")
	require.NoError(t, err)

	assert.Contains(t, script, `readlink -f -- "$TARGET_PATH"`)
	assert.Contains(t, script, `chmod 444 -- "$TMP_PATH"`)
	assert.Contains(t, script, `chown 'www-data':'www-data' -- "$TMP_PATH"`)
	assert.Contains(t, script, `chown 'www-data':'www-data' -- "$BACKUP_PATH"`)
	assert.Contains(t, script, `mv -f -- "$TMP_PATH" "$TARGET_PATH"`)
	assert.Contains(t, script, `trap 'rm -f -- "$TMP_PATH"' EXIT`)

	// 临时文件必须与目标同目录，否则 mv 跨文件系统退化为「拷贝 + 删除」而不再原子
	assert.Contains(t, script, `TMP_PATH="${TARGET_PATH}.tmp.$$.${RANDOM}"`)

	orderOf := func(needle string) int {
		idx := strings.Index(script, needle)
		require.NotEqual(t, -1, idx, "script should contain %q", needle)
		return idx
	}
	assert.Less(t, orderOf("readlink -f"), orderOf("BACKUP_PATH="))
	assert.Less(t, orderOf(`trap 'rm -f`), orderOf("BACKUP_PATH="))
	assert.Less(t, orderOf("base64 -d"), orderOf("chmod 444"))
	assert.Less(t, orderOf("chmod 444"), orderOf(`chown 'www-data':'www-data' -- "$TMP_PATH"`))
	assert.Less(t, orderOf(`chown 'www-data':'www-data' -- "$TMP_PATH"`), orderOf("mv -f --"))
}

// TestBuildLinuxPushScriptPreservesSELinuxContext 原子替换换了 inode，新文件只能拿到目录的默认
// SELinux 标签；原文件若被 chcon 定制过就会退化，业务进程会在下发「成功」后读不到配置。
// 因此必须在替换前把原标签复制到临时文件上，且该步骤在无 SELinux 的机器上要能整段跳过。
func TestBuildLinuxPushScriptPreservesSELinuxContext(t *testing.T) {
	builder := &ScriptBuilder{FileMode: table.Unix}

	script, err := builder.BuildConfigPushScript("Y29udGVudA==", "/etc/app/app.conf", "644", "root", "root")
	require.NoError(t, err)

	assert.Contains(t, script, `chcon --reference="$TARGET_PATH" -- "$TMP_PATH"`)
	// 目标文件不存在（首次下发）或机器未装 chcon 时跳过，避免无谓报错
	assert.Contains(t, script, `if [ -e "$TARGET_PATH" ] && command -v chcon >/dev/null 2>&1; then`)
	// 失败不得中断下发：SELinux 关闭的机器上 chcon 会报错退出
	assert.Contains(t, script, `chcon --reference="$TARGET_PATH" -- "$TMP_PATH" 2>/dev/null || true`)

	chconAt := strings.Index(script, "chcon --reference")
	mvAt := strings.Index(script, "mv -f --")
	chownAt := strings.Index(script, `chown 'root':'root' -- "$TMP_PATH"`)
	require.NotEqual(t, -1, chconAt)
	require.NotEqual(t, -1, mvAt)
	require.NotEqual(t, -1, chownAt)

	// 必须作用在临时文件上并早于替换，否则标签设不到最终文件上
	assert.Less(t, chownAt, chconAt, "chcon 应在 chown 之后，避免属主变更影响标签")
	assert.Less(t, chconAt, mvAt, "chcon 必须早于 mv")
}

// TestLinuxPushScriptSucceedsWithoutSELinuxTooling 构建机通常没有 chcon，
// 该用例确认标签保留步骤不会让下发在这类机器上失败。
func TestLinuxPushScriptSucceedsWithoutSELinuxTooling(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.conf")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))

	out, err := runPushScript(t, target, "new content", "644", 5)
	require.NoError(t, err, "script output: %s", out)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(got))

	assertNoTempLeftover(t, dir)
}

// currentOwner 返回当前进程的用户名与组名。
// 非 root 下 chown 到自身 uid/gid 是允许的，因此脚本执行类用例无需 root 也能跑。
func currentOwner(t *testing.T) (string, string) {
	t.Helper()

	u, err := user.Current()
	require.NoError(t, err)

	g, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)

	return u.Username, g.Name
}

// runPushScript 生成并执行下发脚本，返回脚本输出
func runPushScript(t *testing.T, targetPath, content, fileMode string, maxBackups int) (string, error) {
	t.Helper()

	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	owner, group := currentOwner(t)
	builder := &ScriptBuilder{FileMode: table.Unix, MaxBackups: maxBackups}
	script, err := builder.BuildConfigPushScript(
		base64.StdEncoding.EncodeToString([]byte(content)), targetPath, fileMode, owner, group)
	require.NoError(t, err)

	scriptPath := filepath.Join(t.TempDir(), "push.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o700))

	out, err := exec.Command("bash", scriptPath).CombinedOutput()
	return string(out), err
}

// TestLinuxPushScriptReplacesReadOnlyFile 覆盖故障场景：目标文件为 444 只读。
// 旧实现用 `> "$TARGET_PATH"` 直写，会被文件的只读位挡住；原子替换只改目录项，因此能成功。
func TestLinuxPushScriptReplacesReadOnlyFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.conf")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o444))

	out, err := runPushScript(t, target, "new content", "444", 5)
	require.NoError(t, err, "script output: %s", out)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(got))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o444), info.Mode().Perm())

	assertNoTempLeftover(t, dir)
}

// TestLinuxPushScriptFollowsSymlink 目标为软链接时应更新其指向的真实文件，且软链接本身保留
func TestLinuxPushScriptFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "app.conf.real")
	linkPath := filepath.Join(dir, "app.conf")
	require.NoError(t, os.WriteFile(realPath, []byte("old"), 0o644))
	require.NoError(t, os.Symlink(realPath, linkPath))

	out, err := runPushScript(t, linkPath, "new content", "644", 5)
	require.NoError(t, err, "script output: %s", out)

	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "symlink should be preserved, not replaced by a regular file")

	got, err := os.ReadFile(realPath)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(got))
}

// TestLinuxPushScriptKeepsMaxBackups 备份份数超过上限后应删除最旧的
func TestLinuxPushScriptKeepsMaxBackups(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.conf")
	require.NoError(t, os.WriteFile(target, []byte("v0"), 0o644))

	const maxBackups = 2
	for i := range maxBackups + 2 {
		if i > 0 {
			// 备份文件名按秒取时间戳，同一秒内的多次下发会复用同一个文件名
			time.Sleep(1100 * time.Millisecond)
		}
		out, err := runPushScript(t, target, "v"+strconv.Itoa(i+1), "644", maxBackups)
		require.NoError(t, err, "script output: %s", out)
	}

	backups, err := filepath.Glob(filepath.Join(dir, "app.conf.*.bak"))
	require.NoError(t, err)
	assert.Len(t, backups, maxBackups)
}

// TestLinuxPushScriptKeepsTargetIntactOnChownFailure 覆盖 chown 失败：
// 属主设置在临时文件上，失败时目标文件必须保持原状且不留临时文件残留。
func TestLinuxPushScriptKeepsTargetIntactOnChownFailure(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "app.conf")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))

	builder := &ScriptBuilder{FileMode: table.Unix, MaxBackups: 5}
	script, err := builder.BuildConfigPushScript(
		base64.StdEncoding.EncodeToString([]byte("new content")), target, "644",
		"bscp-user-does-not-exist", "bscp-group-does-not-exist")
	require.NoError(t, err)

	scriptPath := filepath.Join(t.TempDir(), "push.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o700))

	out, err := exec.Command("bash", scriptPath).CombinedOutput()
	require.Error(t, err, "script should fail when the owner does not exist, output: %s", out)

	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(got), "target file must keep its original content")

	assertNoTempLeftover(t, dir)
}

// TestBuildWindowsPushScriptUsesUniqueTempPath 同一台主机上多个任务下发同名配置很常见
// （若干个 .NET 服务各有一份 Web.config，只是目录不同）。这些任务曾经共用
// %TEMP%\bscp_<文件名>_%RANDOM% 这一个命名空间，而 cmd 的 %RANDOM% 在进程启动时以系统时钟
// 播种，被 GSE 同时拉起的脚本会取到相同的值：它们互相覆盖 base64 中转文件、互相删除解码
// 产物，表现为 certutil 报 ERROR_FILE_EXISTS 或 move 找不到文件，甚至静默写入他人的内容。
func TestBuildWindowsPushScriptUsesUniqueTempPath(t *testing.T) {
	builder := &ScriptBuilder{FileMode: table.Windows}
	build := func(absPath string) string {
		script, err := builder.BuildConfigPushScript(
			"Y29udGVudA==", absPath, "", "Administrator", "Administrators")
		require.NoError(t, err)
		return script
	}

	first := build(`D:\bdo\Tmp\GB.BlackDesert.Trade.Web\Web.config`)
	second := build(`D:\bdo\Tmp\GB.BlackDesert.Trade.Web.Game\Web.config`)
	// 同一目标路径重复构建（重试、或同一配置并发下发）同样不能复用临时文件名
	third := build(`D:\bdo\Tmp\GB.BlackDesert.Trade.Web\Web.config`)

	assert.NotContains(t, first, "%RANDOM%", "唯一性不能依赖 cmd 的 %RANDOM%")
	assert.NotContains(t, first, "%TEMP%",
		"临时文件必须与目标同目录：%TEMP% 全机共享，且跨卷 move 不是原子操作")

	firstTmp, firstOut := windowsTempPaths(t, first)
	secondTmp, secondOut := windowsTempPaths(t, second)
	thirdTmp, thirdOut := windowsTempPaths(t, third)

	assert.Contains(t, firstTmp, `!TARGET_DIR!!TARGET_NAME!.`, "临时文件应落在目标同目录")
	assert.NotEqual(t, firstTmp, firstOut, "中转文件与解码产物不能同名")
	assert.NotEqual(t, firstTmp, secondTmp)
	assert.NotEqual(t, firstOut, secondOut)
	assert.NotEqual(t, firstTmp, thirdTmp)
	assert.NotEqual(t, firstOut, thirdOut)
}

// TestBuildWindowsPushScriptGuardsDecodeAndMkdir 覆盖两处会把错误藏起来的地方：
// certutil 不带 -f 时拒绝覆盖已存在的输出文件；mkdir 失败若不拦截，错误会一路滑到 move。
func TestBuildWindowsPushScriptGuardsDecodeAndMkdir(t *testing.T) {
	builder := &ScriptBuilder{FileMode: table.Windows}

	script, err := builder.BuildConfigPushScript(
		"Y29udGVudA==", `D:\bdo\Tmp\Web_IDIP_API\Web.config`, "", "Administrator", "Administrators")
	require.NoError(t, err)

	assert.Contains(t, script, `certutil -f -decode "!BSCP_TMP!" "!BSCP_OUT!"`)
	assert.NotContains(t, script, `certutil -decode`, "缺少 -f 会报 0x80070050 ERROR_FILE_EXISTS")

	mkdirAt := strings.Index(script, "MKDIR_FAILED")
	decodeAt := strings.Index(script, "certutil -f -decode")
	require.NotEqual(t, -1, mkdirAt, "建目录失败必须当场终止")
	require.NotEqual(t, -1, decodeAt)
	assert.Less(t, mkdirAt, decodeAt)
}

var windowsTempVarRe = regexp.MustCompile(`(?m)^set "(BSCP_TMP|BSCP_OUT)=(.+)"$`)

// windowsTempPaths 取出脚本里声明的 base64 中转文件与解码产物路径
func windowsTempPaths(t *testing.T, script string) (string, string) {
	t.Helper()

	paths := make(map[string]string, 2)
	for _, m := range windowsTempVarRe.FindAllStringSubmatch(script, -1) {
		paths[m[1]] = m[2]
	}
	require.Len(t, paths, 2, "script should declare both BSCP_TMP and BSCP_OUT")

	return paths["BSCP_TMP"], paths["BSCP_OUT"]
}

func assertNoTempLeftover(t *testing.T, dir string) {
	t.Helper()

	leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp.*"))
	require.NoError(t, err)
	assert.Empty(t, leftovers, "temporary files must be cleaned up")
}
