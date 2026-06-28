@echo off
chcp 65001 >nul 2>&1
title Hermes 诊断助手 - 命令行模式
cls

echo ========================================
echo   HERMES 诊断助手 v2.0 (CLI模式)
echo ========================================
echo.

REM Check if python exists
where python >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 未找到Python，请确认已安装
    pause
    exit /b 1
)

REM Run the GUI version in CLI mode (it auto-detects)
python "%~dp0hermes_diagnostic.py"

pause
