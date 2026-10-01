# 从源码构建

环境：Windows 10 1903 或更新版本 / Windows 11，64 位；Go 1.23 或更新版本；PowerShell 5.1 或 PowerShell 7。

仓库只使用 Go 标准库。Windows 图形 API 由系统提供，不需要安装 Python、独立显卡软件或额外运行库。首次构建需要下载 Go 工具链；重新生成图标时需要网络。

```powershell
go test ./... -count=1
go vet ./...
powershell -ExecutionPolicy Bypass -File scripts/build.ps1
```

脚本会生成 `dist/v0.7.7/ProjEZ.exe`、便携 ZIP 和 SHA256 校验文件。这里的 ExecutionPolicy Bypass 只作用于该次脚本进程；不修改系统执行策略。也可以直接运行以下命令：

```powershell
go build -trimpath -ldflags="-H=windowsgui -s -w" -o ProjEZ.exe .
```

`-trimpath` 去除编译时的本机源码路径。发布包不包含用户配置、日志、桌面截图或 Git 历史。

## 图标与着色器

仓库已包含图标资源 `resource_windows_amd64.syso` 与 `assets/*.cso`。更改图标后运行 `generate_resources.bat`，它使用固定版本的 [rsrc v0.10.2](https://github.com/akavel/rsrc/tree/v0.10.2)。更改 HLSL 后在 Windows 上运行：

```powershell
go run tools/compile_shaders.go
```

提交 HLSL、编译后的 CSO 和相应测试。普通构建不需要重新生成这两类资源。

## 验证范围

常规测试包含界面响应检查，使用独立临时配置。真实 GPU 捕获测试需要可交互的 Windows 桌面，默认跳过；需要时在专用测试设备设置 `PF_GPU_TEST=1` 后运行测试。不要在正在演示的电脑上运行硬件压力测试。

GitHub Actions 配置用于检查源码和构建便携包。CI 通过不代表所有投影仪、显示比例和驱动都已通过现场验证。当前公开准备使用 Go 1.26.3；最低 Go 版本由 `go.mod` 声明，未逐版实机验证。

发布前可使用 Python 3 运行 `python scripts/check_public.py --include-dist`，检查常见凭据、本机用户目录与 PNG 元数据。脚本同时检查当前仓库的 Git 历史对象，只输出文件名和问题类别；它不替代人工审核截图，也不能保证识别所有敏感信息。
