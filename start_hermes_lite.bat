@echo off
chcp 65001 >nul 2>&1
title Hermes Agent Lite
cls

echo ========================================
echo   Hermes Agent Lite v1.0
echo ========================================
echo.

REM Check for bundled Python first
if exist "%~dp0python\python.exe" (
    set PY=%~dp0python\python.exe
    echo 使用便携Python
) else if exist "%~dp0python.exe" (
    set PY=%~dp0python.exe
    echo 使用便携Python
) else (
    where python >nul 2>&1
    if %errorlevel% equ 0 (
        set PY=python
        echo 使用系统Python
    ) else (
        echo [错误] 未找到Python
        echo.
        echo 请将此U盘插入装有Python 3.10+的电脑
        echo 或使用捆绑版Python（推荐）
        echo.
        pause
        exit /b 1
    )
)

echo.
"%PY%" "%~dp0hermes_lite.py"
pause
