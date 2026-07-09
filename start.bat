@echo off
chcp 65001 >nul 2>&1
title Hermes Portable v1.2.0
cls

echo ========================================
echo   Hermes Portable v1.2.0
echo ========================================
echo.

echo [1/3] Stopping old instances...
taskkill /f /im hermes.exe >nul 2>&1

echo [2/3] Cleaning old sessions...
for /d %%d in ("%~dp0.hermes_portable" "%~dp0*.hermes_portable" "%~dp0releases\.hermes_portable") do (
    if exist "%%d\sessions" rmdir /s /q "%%d\sessions" >nul 2>&1
)

echo [3/3] Starting...
start "" "%~dp0releases\hermes-v1.2.0.exe"
echo Done.
