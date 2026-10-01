@echo off
setlocal
cd /d "%~dp0"
rem Embed the Explorer/shortcut icon as a Windows PE resource.
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/logo.ico -o resource_windows_amd64.syso
exit /b %errorlevel%
