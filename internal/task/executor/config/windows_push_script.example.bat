@echo off
REM ============================================================================
REM  本文件仅供阅读，不会被 Go 加载，也不会下发到目标机。
REM  生产脚本由 script_builder.go 的 buildWindowsPushScript 用 fmt.Sprintf 现场拼出。
REM  「为什么这么写」见同目录 windows_push_script.md。
REM
REM  这里展示「拼完之后、落到 Windows 上真正执行」的形态：
REM    Go 源码里的 %%%%i          ->  本文件的 %%i
REM    Go 源码里的 %%TARGET_PATH%% ->  本文件的 %TARGET_PATH%
REM    Go 源码里的 %%~dp0         ->  本文件的 %~dp0
REM    Go 源码里的 %s / %d        ->  下面这组示例值
REM
REM  示例值：
REM    目标路径   D:\bnsserver\ManagementWeb\web.config
REM    最大备份   5
REM    属主       Administrator
REM    属组       *S-1-5-32-544（内置 Administrators 的 SID）
REM    临时后缀   a1b2c3d4e5f67890
REM    期望大小   9852
REM ============================================================================
REM
REM  ============================================================================
REM   cmd / bat 语法速查（本脚本用到的）
REM  ============================================================================
REM
REM  【1】注释与回显
REM      REM ...          整行注释（括号块里不要用 ::）
REM      @echo off        关掉命令回显，且本行自己也不打印
REM
REM  【2】变量赋值
REM      set "NAME=value" 字符串赋值；引号防止空格/=/& 被拆开
REM      set /a NAME=1+2  算术赋值；只有 /a 才支持 += -=
REM
REM  【3】变量取值：%VAR% vs !VAR!
REM      %VAR%   解析期展开：整段 () 块进入前就定死，块内改值读不到新值
REM      !VAR!   运行期展开：需要 setlocal enabledelayedexpansion
REM      本脚本凡是「先改后读」的地方一律用 !VAR!
REM
REM  【4】子串与路径修饰符
REM      !VAR:~-1!        最后 1 个字符
REM      !VAR:~0,-1!      去掉最后 1 个字符
REM      %%~dpi           for 变量：盘符+路径（含尾 \)
REM      %%~nxi           for 变量：文件名+扩展名
REM      %%~zi            for 变量：文件字节大小
REM      %~dp0            本 bat 所在目录（含尾 \）——GSE 脚本落地目录
REM
REM  【5】分支与跳转
REM      if exist "f" ...
REM      if !ERRORLEVEL! neq 0 (...多行...)
REM      if not "!A!"=="!B!" (...)
REM      :label / goto label / call :label / goto :eof
REM      exit /b 1        结束本 bat，返回码 1（给 GSE 看）
REM
REM  【6】循环
REM      for %%i in ("path") do ...           解析路径
REM      for /l %%n in (1,1,N) do ...         从 1 到 N
REM      for /f "delims=" %%i in ('cmd') do   捕获外部命令每一行输出
REM
REM  【7】重定向（顺序很重要）
REM      >nul 2>&1        丢掉 stdout 和 stderr
REM      >>"file" echo x  追加写入；重定向必须在 echo 前面
REM                       （写成 echo x > file 会把空格写进文件；
REM                        echo ...1>file 末位数字会被当成句柄）
REM
REM  【8】其它
REM      md dir           创建目录；已存在则 ERRORLEVEL!=0（用来做锁）
REM      ping 127.0.0.1 -n 2 >nul   约睡 1 秒（GSE 下不能用 timeout）
REM      copy /y ... || ( ... )     失败则执行括号块
REM ============================================================================

setlocal enabledelayedexpansion

set "TARGET_PATH=D:\bnsserver\ManagementWeb\web.config"
set /a MAX_BACKUPS=5
set "BSCP_OWNER=Administrator"

REM 0. 校验属主可解析为 SID
powershell -NoProfile -Command "try { [void]([System.Security.Principal.NTAccount]$env:BSCP_OWNER).Translate([System.Security.Principal.SecurityIdentifier]) } catch { exit 1 }"
if !ERRORLEVEL! neq 0 (
    echo OWNER_NOT_FOUND
    exit /b 1
)

set "TARGET_GROUP=*S-1-5-32-544"

REM 1. 解析目录和文件名
REM    输入 TARGET_PATH = D:\bnsserver\ManagementWeb\web.config
REM    解析后：
REM      TARGET_DIR  = D:\bnsserver\ManagementWeb\   （%%~dpi = 盘符+路径，含尾 \）
REM      TARGET_NAME = web.config                    （%%~nxi = 文件名+扩展名）
for %%i in ("%TARGET_PATH%") do (
    set "TARGET_DIR=%%~dpi"
    set "TARGET_NAME=%%~nxi"
)

REM 2. 创建目标目录
set "DIR_CUR=!TARGET_DIR!"
if "!DIR_CUR:~-1!"=="\" set "DIR_CUR=!DIR_CUR:~0,-1!"
set /a NEW_DIR_COUNT=0
:collect_new_dirs
if exist "!DIR_CUR!\" goto collect_new_dirs_done
set /a NEW_DIR_COUNT+=1
set "NEW_DIR_!NEW_DIR_COUNT!=!DIR_CUR!"
for %%i in ("!DIR_CUR!") do set "DIR_PARENT=%%~dpi"
if "!DIR_PARENT:~-1!"=="\" set "DIR_PARENT=!DIR_PARENT:~0,-1!"
if "!DIR_PARENT!"=="!DIR_CUR!" goto collect_new_dirs_done
set "DIR_CUR=!DIR_PARENT!"
goto collect_new_dirs
:collect_new_dirs_done

if not exist "!TARGET_DIR!" mkdir "!TARGET_DIR!"
if not exist "!TARGET_DIR!" (
    echo MKDIR_FAILED
    exit /b 1
)

REM 只给本次新建的层级设属主
for /l %%n in (1,1,!NEW_DIR_COUNT!) do (
    set "NEW_DIR=!NEW_DIR_%%n!"
    icacls "!NEW_DIR!" /setowner "!BSCP_OWNER!" >nul
    if !ERRORLEVEL! neq 0 (
        echo ACL_FAILED
        echo [ERROR] icacls /setowner failed on !NEW_DIR!, errorlevel=!ERRORLEVEL!
        goto cleanup_new_dirs
    )
    icacls "!NEW_DIR!" /grant:r "!BSCP_OWNER!:(F)" >nul
    if !ERRORLEVEL! neq 0 (
        echo ACL_FAILED
        echo [ERROR] icacls grant owner failed on !NEW_DIR!, errorlevel=!ERRORLEVEL!
        goto cleanup_new_dirs
    )
    icacls "!NEW_DIR!" /grant:r "!TARGET_GROUP!:(RX)" >nul
    if !ERRORLEVEL! neq 0 (
        echo ACL_FAILED
        echo [ERROR] icacls grant group failed on !NEW_DIR!, errorlevel=!ERRORLEVEL!
        goto cleanup_new_dirs
    )
)

REM ----------------- 临界区互斥锁 -----------------
set "LOCK_DIR=!TARGET_DIR!!TARGET_NAME!.bscp.lock"
set /a LOCK_WAIT_SEC=0
set /a MAX_LOCK_WAIT=60

:lock_acquire
md "!LOCK_DIR!" >nul 2>&1
if !ERRORLEVEL! equ 0 goto lock_acquired
set /a LOCK_WAIT_SEC+=1
if !LOCK_WAIT_SEC! gtr !MAX_LOCK_WAIT! (
    echo [ERROR] 等待文件锁超时: !LOCK_DIR!
    echo LOCK_TIMEOUT
    exit /b 1
)
ping 127.0.0.1 -n 2 >nul
goto lock_acquire

:lock_acquired

REM ----------------- 临界区开始 -----------------

REM 3. 备份原文件
if exist "!TARGET_PATH!" (
    echo [INFO] 发现原文件，准备备份...

    REM 获取毫秒级时间戳，防止同秒并发覆盖
    for /f "delims=" %%i in (
        'powershell -NoProfile -Command "Get-Date -Format yyyyMMddHHmmssfff"'
    ) do set "STAMP=%%i"

    echo [INFO] 时间戳: !STAMP!

    set "BACKUP_FILE=!TARGET_NAME!.!STAMP!.bscp.a1b2c3d4e5f67890.bak"
    set "BACKUP_FULL_PATH=!TARGET_DIR!!BACKUP_FILE!"

    copy /y "!TARGET_PATH!" "!BACKUP_FULL_PATH!" >nul || (
        echo [ERROR] 备份失败
        goto :fail
    )
    echo [OK] 备份已生成: !BACKUP_FILE!

    REM 统计备份数量
    set /a COUNT=0
    for /f "delims=" %%f in (
        'dir /b /o:d "!TARGET_DIR!!TARGET_NAME!.*.bak" 2^>nul'
    ) do set /a COUNT+=1

    echo [INFO] 当前备份数: !COUNT! / 最大保留: %MAX_BACKUPS%

    REM 删除最旧备份
    if !COUNT! gtr %MAX_BACKUPS% (
        set /a DEL_COUNT=!COUNT!-%MAX_BACKUPS%
        echo [INFO] 需删除最旧备份数: !DEL_COUNT!

        set /a IDX=0
        for /f "delims=" %%f in (
            'dir /b /o:d "!TARGET_DIR!!TARGET_NAME!.*.bak" 2^>nul'
        ) do (
            if !IDX! lss !DEL_COUNT! (
                echo [CLEAN] 删除旧备份: %%f
                del /f /q "!TARGET_DIR!%%f" >nul 2>&1
                set /a IDX+=1
            )
        )
    )
) else (
    echo [INFO] 目标文件不存在，跳过备份。
)

REM 4. 写 base64 中转文件并解码，按期望长度校验，不通过则整份重写
set "BSCP_TMP=%~dp0!TARGET_NAME!.bscp.a1b2c3d4e5f67890.b64"
set "BSCP_OUT=!TARGET_DIR!!TARGET_NAME!.bscp.a1b2c3d4e5f67890.out"
set "EXPECTED_SIZE=9852"
set "WRITE_FAIL=WRITE_TMP_FAILED"
set /a WRITE_TRY=0
set /a MAX_WRITE_TRY=3

:write_b64
set /a WRITE_TRY+=1
del /f /q "!BSCP_TMP!" >nul 2>&1
del /f /q "!BSCP_OUT!" >nul 2>&1
REM --- 服务端在这里插入多行：>>"!BSCP_TMP!" echo <每段最多1024字符的base64> ---
REM --- 示例只写一行占位；真实下发按内容切成 N 段 ---
>>"!BSCP_TMP!" echo SGVsbG8=

if not exist "!BSCP_TMP!" (
    set "WRITE_FAIL=WRITE_TMP_FAILED"
    echo [WARN] base64 中转文件未生成, try=!WRITE_TRY!
    goto write_retry
)

certutil -f -decode "!BSCP_TMP!" "!BSCP_OUT!" >nul
if !ERRORLEVEL! neq 0 (
    set "WRITE_FAIL=DECODE_FAILED"
    echo [WARN] certutil 解码失败, errorlevel=!ERRORLEVEL! try=!WRITE_TRY!
    goto write_retry
)

set "OUT_SIZE="
for %%i in ("!BSCP_OUT!") do set "OUT_SIZE=%%~zi"
if not "!OUT_SIZE!"=="!EXPECTED_SIZE!" (
    set "WRITE_FAIL=WRITE_TMP_TRUNCATED"
    echo [WARN] 解码长度不符: got=!OUT_SIZE! want=!EXPECTED_SIZE! try=!WRITE_TRY!
    goto write_retry
)
goto write_ok

:write_retry
REM 中转文件创建不出来时退回目标同目录；写残则仍在脚本目录里重写
if "!WRITE_FAIL!"=="WRITE_TMP_FAILED" set "BSCP_TMP=!TARGET_DIR!!TARGET_NAME!.bscp.a1b2c3d4e5f67890.b64"
if !WRITE_TRY! lss !MAX_WRITE_TRY! (
    ping 127.0.0.1 -n 2 >nul
    goto write_b64
)
echo !WRITE_FAIL!
echo [ERROR] base64 写盘校验连续 !MAX_WRITE_TRY! 次失败，目标文件保持原内容不变
del /f /q "!BSCP_TMP!" >nul 2>&1
del /f /q "!BSCP_OUT!" >nul 2>&1
goto :fail

:write_ok
echo [OK] 解码产物长度校验通过: !OUT_SIZE! 字节, try=!WRITE_TRY!
del /f /q "!BSCP_TMP!" >nul 2>&1

REM 5. 覆盖到目标文件
set /a MOVE_RETRY=0
:try_move
move /y "!BSCP_OUT!" "%TARGET_PATH%" >nul 2>&1
if !ERRORLEVEL! equ 0 goto move_success
set /a MOVE_RETRY+=1
if !MOVE_RETRY! leq 10 (
    ping 127.0.0.1 -n 2 >nul
    goto try_move
)
echo MOVE_FAILED
del /f /q "!BSCP_OUT!" >nul 2>&1
goto :fail

:move_success

REM 6. 设置文件属主与权限
icacls "%TARGET_PATH%" /setowner "!BSCP_OWNER!" >nul
if !ERRORLEVEL! neq 0 (
    echo ACL_FAILED
    echo [ERROR] icacls /setowner failed, errorlevel=!ERRORLEVEL!
    goto :fail
)
icacls "%TARGET_PATH%" /grant:r "!BSCP_OWNER!:(F)" >nul
if !ERRORLEVEL! neq 0 (
    echo ACL_FAILED
    echo [ERROR] icacls grant owner full control failed, errorlevel=!ERRORLEVEL!
    goto :fail
)
icacls "%TARGET_PATH%" /grant:r "!TARGET_GROUP!:(R)" >nul
if !ERRORLEVEL! neq 0 (
    echo ACL_FAILED
    echo [ERROR] icacls grant group read failed, errorlevel=!ERRORLEVEL!
    goto :fail
)

REM 7. 校验
dir "%TARGET_PATH%"
certutil -hashfile "%TARGET_PATH%" MD5

REM 释放锁并正常结束
call :release_lock
endlocal
goto :eof

REM ----------------- 异常处理标签 -----------------
:fail
call :release_lock
endlocal
exit /b 1

:release_lock
rmdir "!LOCK_DIR!" >nul 2>&1
goto :eof

:cleanup_new_dirs
for /l %%n in (1,1,!NEW_DIR_COUNT!) do (
    set "NEW_DIR=!NEW_DIR_%%n!"
    rmdir "!NEW_DIR!" >nul 2>&1
)
endlocal
exit /b 1
