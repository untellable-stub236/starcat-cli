# starcat-cli

`starcat-cli` 是 Starcat 的跨平台命令行客户端，也是 Codex、Claude Code 等 AI Agent 可使用的 stdio MCP Server。

CLI 不读取 Starcat 数据库，也不复制 App 的业务逻辑。所有读取和写入统一调用 Starcat 提供的 MCP Tools，因此权限、dry-run、Pro 校验和审计仍由 Starcat 控制。

## 支持平台

- macOS `arm64` / `amd64`
- Linux `arm64` / `amd64`
- Windows `amd64`

Starcat App 仍运行在 macOS；CLI 可以运行在同一台 Mac，也可以从可信局域网或 Tailscale/WireGuard 网络中的其他设备连接。

## 配对

在 Starcat 的「设置 → MCP 服务」中开启服务并点击「配对新设备」，然后执行复制出的命令：

```bash
starcat pair "starcat-pair://connect?..."
starcat doctor --json
```

配对 URI 五分钟后过期且只能使用一次。长期设备凭据保存在 macOS Keychain、Windows Credential Manager 或 Linux Secret Service，不写入项目文件。

## 作为 MCP Server

完成配对后，将 Agent 的 MCP Server 配置为：

```json
{
  "command": "/absolute/path/to/starcat",
  "args": ["mcp"]
}
```

`starcat mcp` 将 stdin/stdout JSON-RPC 原样桥接到 Starcat 的 MCP Streamable HTTP endpoint。stdout 只输出 MCP 协议消息，诊断信息写入 stderr。

## 常用命令

```bash
starcat capabilities --json
starcat repo search "local RAG" --scope starred --limit 20
starcat repo context owner/repo
starcat repo readme owner/repo
starcat repo summary owner/repo
starcat tags list
```

写操作默认只 dry-run，必须显式添加 `--apply`：

```bash
printf '%s' '一段私有笔记' | starcat repo note set owner/repo --stdin
printf '%s' '一段私有笔记' | starcat repo note set owner/repo --stdin --apply
starcat repo tags add owner/repo Swift macOS --apply
starcat repo status set owner/repo using --apply
```

## 本地开发

```bash
go test ./...
go vet ./...
go build -o bin/starcat ./cmd/starcat
```

发布构建通过 `-ldflags` 注入版本：

```bash
go build -ldflags "-X github.com/dong4j/starcat-cli/internal/mcp.Version=v0.1.0" ./cmd/starcat
```

维护者可以使用 `scripts/build-all.sh v0.1.0` 生成五个平台二进制和 `checksums.txt`。脚本不创建 tag，也不上传 GitHub Release。

## 安全边界

- 明文 HTTP 只允许连接 `localhost` / loopback。
- 远程连接必须使用 HTTPS，并 pin 配对 URI 中的 SHA-256 证书指纹。
- 每台设备使用独立、可撤销的 token。
- CLI 不在参数、stdout 或日志中输出长期 token。
- Starcat 设置中的 MCP 写权限仍是最终授权边界。

## License

MIT
