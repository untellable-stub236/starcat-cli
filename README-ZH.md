# Starcat CLI

`starcat` 是 Starcat 的跨平台命令行客户端，也是 Codex、Claude Code 等 AI Agent 可使用的 stdio MCP Server。

CLI 不直接读取 Starcat 数据库，也不复制 App 的业务逻辑。读取与写入统一通过 Starcat MCP Tools 完成，因此权限、dry-run、Pro 校验和审计仍由 Starcat App 控制。

[English](./README.md)

## 支持平台

- macOS：`arm64`、`amd64`
- Linux：`arm64`、`amd64`
- Windows：`amd64`

Starcat App 仍运行在 macOS；CLI 可以运行在同一台 Mac，也可以从可信局域网、Tailscale 或 WireGuard 设备连接。

## 安装

### Homebrew

```bash
brew tap starcat-app/starcat-cli
brew install starcat
```

Tap 仓库名为 `starcat-app/homebrew-starcat-cli`，安装后的命令仍是 `starcat`。

### macOS / Linux 一键安装

```bash
curl -fsSL https://github.com/starcat-app/starcat-cli/releases/latest/download/install.sh | sh
```

默认安装到 `~/.local/bin/starcat`，可通过 `STARCAT_INSTALL_DIR` 覆盖。安装脚本会显示平台检测、下载、SHA-256 校验和安装进度；完成后给出 PATH、配对步骤和常用命令。

### Windows PowerShell

```powershell
irm https://github.com/starcat-app/starcat-cli/releases/latest/download/install.ps1 | iex
```

默认安装到 `$HOME\.local\bin\starcat.exe`。PowerShell 安装脚本同样会显示版本解析、平台检测、下载、SHA-256 校验和安装进度，并在完成后给出 PATH、配对步骤和常用命令。

## 配对

在 Starcat 的「设置 → MCP 服务」中启动服务，点击「复制配对命令」，把完整命令粘贴到目标设备的终端并按回车，然后回到 Starcat 确认设备：

```bash
starcat pair "starcat-pair://connect?..."
starcat doctor
```

需要手工输入时，可运行 `starcat pair`，粘贴 URI 后直接按回车。配对命令五分钟内有效、只能兑换一次，并且仍需在 Starcat 中人工确认。长期设备凭据保存在 macOS Keychain、Windows Credential Manager 或 Linux Secret Service。

## 更新

CLI 每 24 小时最多检查一次 GitHub Release，只在交互式终端显示英文更新提示；`starcat mcp` 和自动化管道不会输出更新提示。

```bash
starcat update
```

Homebrew 安装请使用：

```bash
brew update
brew upgrade starcat
```

如需关闭自动检查：

```bash
export STARCAT_NO_UPDATE_CHECK=1
```

## 常用命令

```bash
starcat help
starcat capabilities
starcat stats
starcat stats ai --range 30d
starcat stats knowledge
starcat repo search "local RAG" --scope starred --limit 20
starcat repo context owner/repo
starcat repo readme owner/repo
starcat repo summary owner/repo
starcat tags list
```

`starcat stats`、`starcat stats ai`、`starcat stats knowledge` 使用适合终端阅读的输出，并且故意不提供 `--json`。Agent 通过 `starcat mcp` 调用同一组统计工具，直接获得结构化结果。统计全部来自本地只读数据：概览覆盖 Star、知识库、AI Token 和 RAG 分片；AI 用量支持时间、功能、Provider、Model 筛选；知识库统计包含来源覆盖与索引健康度。

`help`、`version`、`pair`、`unpair`、`doctor` 和 `update` 默认使用适合终端阅读的文本；`capabilities`、`repo`、`tags` 等数据命令直接输出 JSON。只有自动化确实需要机器可读诊断时才使用 `starcat doctor --json`。

写操作默认 dry-run，必须显式传 `--apply` 才会持久化。

## 开发与贡献

需要 Go 1.25 或更高版本。模块将 Release 构建工具链固定为 Go 1.26.5 或更高版本，确保发布产物包含当前标准库安全修复。开发、测试、安全边界和贡献要求分别见 [README.md](./README.md)、[SECURITY.md](./SECURITY.md) 与 [CONTRIBUTING.md](./CONTRIBUTING.md)。

## License

MIT。二进制分发同时包含 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。
