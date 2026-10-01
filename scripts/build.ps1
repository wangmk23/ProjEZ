param([ValidatePattern('^\d+\.\d+\.\d+$')][string]$Version = '0.7.7')
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$outputRoot = Join-Path $projectRoot 'dist'
$packageDir = Join-Path $outputRoot "v$Version"
$originalEnv = @{ GOOS=$env:GOOS; GOARCH=$env:GOARCH; CGO_ENABLED=$env:CGO_ENABLED }
Push-Location -LiteralPath $projectRoot
try {
    # Version labels must agree with the embedded application version.
    $appSource = Get-Content -LiteralPath (Join-Path $projectRoot 'settings.go') -Raw
    if ($appSource -notmatch ('v' + [regex]::Escape($Version))) { throw 'Version does not match settings.go.' }
    New-Item -ItemType Directory -Force -Path $packageDir | Out-Null
    $env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
    if (-not (Test-Path -LiteralPath 'resource_windows_amd64.syso')) {
        & go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/logo.ico -o resource_windows_amd64.syso
        if ($LASTEXITCODE -ne 0) { throw 'Icon resource generation failed.' }
    }
    $exePath = Join-Path $packageDir 'ProjEZ.exe'
    & go build -trimpath '-ldflags=-H=windowsgui -s -w' -o $exePath .
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
    foreach ($name in @('LICENSE','THIRD_PARTY_NOTICES.md')) {
        Copy-Item -LiteralPath (Join-Path $projectRoot $name) -Destination (Join-Path $packageDir $name) -Force
    }
    $readme = @"
# ProjEZ $Version

Windows 10 1903+ / Windows 11 x64 便携版。解压后运行 ProjEZ.exe。

本产品由 AI 编制，功能与改进方向来自使用者的需求、测试和反馈。

1. 连接投影仪，Windows 选择“扩展”，将操作屏设为主屏。
2. 在程序中选择来源和观众屏幕，或选择应用窗口，再开始投影。
3. 冻结后主屏可以继续操作；需要隐藏内容时使用黑屏。

冻结 Ctrl+Alt+F8；恢复 Ctrl+Alt+F9；黑屏 Ctrl+Alt+F10；停止 Ctrl+Alt+F12。
前四项可在快捷键页面直接按组合键录制并保存。Ctrl+Alt+F11 释放鼠标限制。

停止会关闭输出窗口，可能露出扩展屏桌面。请勿把停止当作黑屏。
程序未签名，请核对 SHA256。不要关闭安全防护来绕过拦截。
设置保存在本机 APPDATA/ProjectorFreezer/config.ini，不自动上传画面。

完整说明与源码：https://github.com/wangmk23/ProjEZ
项目许可见 LICENSE，第三方许可见 THIRD_PARTY_NOTICES.md。
"@
    [IO.File]::WriteAllText((Join-Path $packageDir 'README.md'), $readme, [Text.UTF8Encoding]::new($false))
    $guide = @"
ProjEZ $Version / Windows x64

This product was developed by AI, with its features and improvements guided by user requirements, testing, and feedback.

Run ProjEZ.exe. Set Windows displays to Extend, then select the source and audience screen.
Freeze: Ctrl+Alt+F8 | Resume: Ctrl+Alt+F9 | Black: Ctrl+Alt+F10 | Stop: Ctrl+Alt+F12
Release cursor: Ctrl+Alt+F11. The first four shortcuts can be recorded in Settings.
Stop closes the output window and can reveal the extended desktop. Use Black to hide content.
Settings are stored locally under APPDATA/ProjectorFreezer/config.ini.
Documentation: https://github.com/wangmk23/ProjEZ
Unsigned build. Verify SHA256 and download origin; do not disable security protection.
"@
    $guidePath = Join-Path $packageDir 'QUICKSTART.txt'
    [IO.File]::WriteAllText($guidePath, $guide, [Text.UTF8Encoding]::new($false))
    $exeHash = (Get-FileHash -LiteralPath $exePath -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText((Join-Path $packageDir 'SHA256SUMS.txt'), "$exeHash  ProjEZ.exe`n", [Text.UTF8Encoding]::new($false))
    $zipPath = Join-Path $outputRoot "ProjEZ-$Version-windows-x64.zip"
    # Explicit allowlist excludes any stale personal files in an existing output directory.
    $packageFiles = @('ProjEZ.exe','LICENSE','README.md','THIRD_PARTY_NOTICES.md','QUICKSTART.txt','SHA256SUMS.txt') | ForEach-Object { Join-Path $packageDir $_ }
    Compress-Archive -LiteralPath $packageFiles -DestinationPath $zipPath -Force
    $zipHash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText((Join-Path $outputRoot 'SHA256SUMS.txt'), "$zipHash  $([IO.Path]::GetFileName($zipPath))`n$exeHash  v$Version/ProjEZ.exe`n", [Text.UTF8Encoding]::new($false))
    Write-Output "Built: $zipPath"
    Write-Output "EXE SHA256: $exeHash"
} finally {
    foreach ($name in $originalEnv.Keys) { [Environment]::SetEnvironmentVariable($name, $originalEnv[$name], 'Process') }
    Pop-Location
}
