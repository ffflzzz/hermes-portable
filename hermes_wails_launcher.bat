@echo off
chcp 65001 >nul 2>&1
title Hermes 诊断助手 (Wails 版)
cls
echo ========================================
echo   Hermes 诊断助手 - 便携版 (Wails)
echo ========================================
echo.
echo 正在启动...
echo.
"%~dp0hermes_wails\build\bin\hermes.exe"
