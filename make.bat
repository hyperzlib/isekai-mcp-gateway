@echo off
rem Run make.ps1 with Bypass execution policy, forwarding all arguments
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0make.ps1" %*
exit /b %ERRORLEVEL%
