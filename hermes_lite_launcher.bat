@echo off
chcp 65001 >nul 2>&1
title Hermes Agent Lite
cls

echo ========================================
echo   Hermes Agent Lite v1.0
echo ========================================
echo.

REM Find Python in common locations
set PYTHON_PATH=

REM Check if portable Python exists
if exist "%~dp0python\python.exe" (
    set PYTHON_PATH=%~dp0python\python.exe
) else if exist "%~dp0python.exe" (
    set PYTHON_PATH=%~dp0python.exe
) else (
    where python >nul 2>&1 && set PYTHON_PATH=python
)

if "%PYTHON_PATH%"=="" (
    echo [错误] 未找到Python
    echo.
    echo 请确保U盘中包含Python便携版
    echo 或安装Python 3.10+到目标电脑
    echo.
    pause
    exit /b 1
)

echo 正在启动...
echo Python: %PYTHON_PATH%
echo.

"%PYTHON_PATH%" "%~dp0hermes_lite.py"

pause
