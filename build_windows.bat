@echo off
setlocal
cd /d "%~dp0"
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
if not exist resource_windows_amd64.syso (
  call generate_resources.bat
  if errorlevel 1 exit /b 1
)
go build -trimpath -ldflags="-H=windowsgui -s -w" -o ProjEZ.exe .
if errorlevel 1 exit /b 1
echo.
echo Built: %CD%\ProjEZ.exe
pause

