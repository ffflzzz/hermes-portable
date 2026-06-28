@echo off
chcp 65001 >nul 2>&1
title Hermes 诊断助手 - 命令行模式
cls

echo ========================================
echo   Hermes 诊断助手 v2.0
echo ========================================
echo.
echo 正在启动命令行模式...
echo.

REM Find python in common locations
set PYTHON_PATH=
where python >nul 2>&1 && set PYTHON_PATH=python

if "%PYTHON_PATH%"=="" (
    echo [错误] 未找到Python
    echo 请先安装Python 3.10+
    echo https://www.python.org/downloads/
    pause
    exit /b 1
)

REM Run CLI
%PYTHON_PATH% "%~dp0hermes_cli.py"

pause
