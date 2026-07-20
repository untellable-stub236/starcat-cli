// Package cli 提供 starcat 命令入口。
// 命令只做参数校验与 MCP Tool 映射，写操作默认 dry-run，必须显式传 --apply。
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/starcat-app/starcat-cli/internal/config"
	"github.com/starcat-app/starcat-cli/internal/credential"
	"github.com/starcat-app/starcat-cli/internal/mcp"
	"github.com/starcat-app/starcat-cli/internal/pairing"
	"github.com/starcat-app/starcat-cli/internal/updater"
)

const (
	maxNoteBytes       = 1 << 20
	maxPairingURIBytes = 16 << 10
)

// Runner 持有可替换依赖，生产入口和测试共享完全相同的命令路径。
type Runner struct {
	Profiles    config.Store
	Credentials credential.Store
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	RunUpdate   func(context.Context, string) (updater.Result, error)

	// stdinInteractive 只控制人类可见提示；stdout 仍严格保留给 JSON/MCP 输出。
	stdinInteractive bool
}

func NewRunner(stdin io.Reader, stdout, stderr io.Writer) (*Runner, error) {
	profiles, err := config.DefaultFileStore()
	if err != nil {
		return nil, err
	}
	return &Runner{
		Profiles:         profiles,
		Credentials:      credential.KeyringStore{},
		Stdin:            stdin,
		Stdout:           stdout,
		Stderr:           stderr,
		RunUpdate:        updater.NewClient().Update,
		stdinInteractive: isInteractiveReader(stdin),
	}, nil
}

func (r *Runner) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return r.writeJSON(map[string]any{"help": usage})
	}
	switch args[0] {
	case "help", "--help", "-h":
		return r.writeJSON(map[string]any{"help": usage})
	case "version", "--version":
		return r.writeJSON(map[string]any{"version": mcp.Version})
	case "pair":
		return r.runPair(ctx, args[1:])
	case "unpair":
		return r.unpair()
	case "update":
		if len(args) != 1 {
			return errors.New("Usage: starcat update")
		}
		if r.RunUpdate == nil {
			return errors.New("the update service is unavailable")
		}
		result, err := r.RunUpdate(ctx, mcp.Version)
		if err != nil {
			return err
		}
		return r.writeJSON(result)
	case "doctor":
		return r.doctor(ctx)
	case "capabilities":
		return r.call(ctx, "starcat.get_capabilities", map[string]any{})
	case "mcp":
		transport, err := r.loadTransport()
		if err != nil {
			return err
		}
		return mcp.BridgeStdio(ctx, transport, r.Stdin, r.Stdout)
	case "repo":
		return r.runRepo(ctx, args[1:])
	case "tags":
		if len(args) == 2 && args[1] == "list" {
			return r.call(ctx, "starcat.list_tags", map[string]any{})
		}
		return errors.New("Usage: starcat tags list")
	case "tag":
		return r.runTag(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q; run `starcat help` for usage", args[0])
	}
}

func (r *Runner) runPair(ctx context.Context, args []string) error {
	if len(args) != 1 || args[0] != "--stdin" {
		return errors.New("Usage: starcat pair --stdin")
	}
	if r.stdinInteractive && r.Stderr != nil {
		fmt.Fprintln(r.Stderr, pairingInputPrompt())
	}
	rawURI, err := readAllContext(ctx, io.LimitReader(r.Stdin, maxPairingURIBytes+1))
	if err != nil {
		return fmt.Errorf("read pairing URI from stdin: %w", err)
	}
	if len(rawURI) > maxPairingURIBytes {
		return fmt.Errorf("pairing URI exceeds the %d-byte limit", maxPairingURIBytes)
	}
	profile, err := (pairing.Service{Profiles: r.Profiles, Credentials: r.Credentials}).Pair(ctx, string(rawURI))
	if err != nil {
		return err
	}
	return r.writeJSON(map[string]any{
		"paired":           true,
		"device_id":        profile.DeviceID,
		"endpoint":         profile.Endpoint,
		"protocol_version": profile.ProtocolVersion,
	})
}

func (r *Runner) runRepo(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing repo subcommand")
	}
	switch args[0] {
	case "search":
		positionals, flags, err := parseFlags(args[1:], map[string]bool{"scope": true, "limit": true})
		if err != nil {
			return err
		}
		if len(positionals) != 1 {
			return errors.New("Usage: starcat repo search <query> [--scope starred|knowledge|all] [--limit N] [--semantic]")
		}
		limit, err := intFlag(flags, "limit", 20, 1, 100)
		if err != nil {
			return err
		}
		scope := valueFlag(flags, "scope", "starred")
		if scope != "starred" && scope != "knowledge" && scope != "all" {
			return errors.New("--scope must be starred, knowledge, or all")
		}
		tool := "starcat.search_repos"
		if hasFlag(flags, "semantic") {
			tool = "starcat.semantic_search"
		}
		return r.call(ctx, tool, map[string]any{"query": positionals[0], "scope": scope, "limit": limit})
	case "context", "readme", "summary":
		positionals, flags, err := parseFlags(args[1:], nil)
		if err != nil {
			return err
		}
		if len(positionals) != 1 {
			return fmt.Errorf("Usage: starcat repo %s <owner/name>", args[0])
		}
		selector, err := repoSelector(positionals[0])
		if err != nil {
			return err
		}
		tool := map[string]string{
			"context": "starcat.get_repo_context",
			"readme":  "starcat.get_readme",
			"summary": "starcat.get_repo_summary",
		}[args[0]]
		if args[0] == "summary" && hasFlag(flags, "generate") {
			tool = "starcat.generate_repo_summary"
			selector["allow_external_context"] = hasFlag(flags, "allow-external-context")
		}
		return r.call(ctx, tool, selector)
	case "note":
		return r.runRepoNote(ctx, args[1:])
	case "status":
		return r.runRepoStatus(ctx, args[1:])
	case "tags":
		return r.runRepoTags(ctx, args[1:])
	default:
		return fmt.Errorf("unknown repo subcommand %q", args[0])
	}
}

func (r *Runner) runRepoNote(ctx context.Context, args []string) error {
	positionals, flags, err := parseFlags(args, nil)
	if err != nil {
		return err
	}
	if len(positionals) != 2 || positionals[0] != "set" || !hasFlag(flags, "stdin") {
		return errors.New("Usage: starcat repo note set <owner/name> --stdin [--apply]")
	}
	selector, err := repoSelector(positionals[1])
	if err != nil {
		return err
	}

	// 必须在读取 stdin 前预检权限，否则交互式终端会先无限等待正文，用户永远看不到
	// Starcat 已关闭写入的明确提示。
	client, err := r.writeClient(ctx, "local_writes")
	if err != nil {
		return err
	}
	if r.stdinInteractive && r.Stderr != nil {
		fmt.Fprintln(r.Stderr, interactiveNotePrompt())
	}
	content, err := readAllContext(ctx, io.LimitReader(r.Stdin, maxNoteBytes+1))
	if err != nil {
		return fmt.Errorf("read note from stdin: %w", err)
	}
	if len(content) > maxNoteBytes {
		return errors.New("note exceeds the 1 MiB limit")
	}
	selector["content"] = string(content)
	selector["dry_run"] = !hasFlag(flags, "apply")
	return r.callClient(ctx, client, "starcat.upsert_repo_note", selector)
}

func (r *Runner) runRepoStatus(ctx context.Context, args []string) error {
	positionals, flags, err := parseFlags(args, nil)
	if err != nil {
		return err
	}
	if len(positionals) != 3 || positionals[0] != "set" {
		return errors.New("Usage: starcat repo status set <owner/name> <unread|read|using> [--apply]")
	}
	status := positionals[2]
	if status != "unread" && status != "read" && status != "using" {
		return errors.New("status must be unread, read, or using")
	}
	selector, err := repoSelector(positionals[1])
	if err != nil {
		return err
	}
	selector["status"] = status
	selector["dry_run"] = !hasFlag(flags, "apply")
	return r.callWrite(ctx, "starcat.set_repo_status", "local_writes", selector)
}

func (r *Runner) runRepoTags(ctx context.Context, args []string) error {
	positionals, flags, err := parseFlags(args, nil)
	if err != nil {
		return err
	}
	if len(positionals) < 3 {
		return errors.New("Usage: starcat repo tags <add|remove|replace> <owner/name> <tag...> [--apply]")
	}
	action := positionals[0]
	tool, ok := map[string]string{
		"add":     "starcat.add_repo_tags",
		"remove":  "starcat.remove_repo_tags",
		"replace": "starcat.set_repo_tags",
	}[action]
	if !ok {
		return errors.New("tag action must be add, remove, or replace")
	}
	selector, err := repoSelector(positionals[1])
	if err != nil {
		return err
	}
	selector["tags"] = positionals[2:]
	selector["dry_run"] = !hasFlag(flags, "apply")
	if action != "remove" {
		selector["create_missing"] = true
	}
	capability := "local_writes"
	if action == "replace" {
		capability = "destructive_writes"
	}
	return r.callWrite(ctx, tool, capability, selector)
}

func (r *Runner) runTag(ctx context.Context, args []string) error {
	positionals, flags, err := parseFlags(args, map[string]bool{"color": true, "icon": true})
	if err != nil {
		return err
	}
	if len(positionals) != 2 || positionals[0] != "create" {
		return errors.New("Usage: starcat tag create <name> [--color '#0A84FF'] [--icon tag] [--apply]")
	}
	arguments := map[string]any{"name": positionals[1], "dry_run": !hasFlag(flags, "apply")}
	if value := valueFlag(flags, "color", ""); value != "" {
		arguments["color"] = value
	}
	if value := valueFlag(flags, "icon", ""); value != "" {
		arguments["icon"] = value
	}
	return r.callWrite(ctx, "starcat.create_tag", "local_writes", arguments)
}

func (r *Runner) doctor(ctx context.Context) error {
	profile, err := r.Profiles.Load()
	if err != nil {
		return err
	}
	client, err := r.loadClient()
	if err != nil {
		return err
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return err
	}
	capabilities, err := client.CallTool(ctx, "starcat.get_capabilities", map[string]any{})
	if err != nil {
		return err
	}
	return r.writeJSON(map[string]any{
		"healthy":          true,
		"cli_version":      mcp.Version,
		"app_version":      profile.AppVersion,
		"protocol_version": profile.ProtocolVersion,
		"endpoint":         profile.Endpoint,
		"tool_count":       len(tools),
		"capabilities":     capabilities,
	})
}

func (r *Runner) unpair() error {
	profile, err := r.Profiles.Load()
	if err != nil && !errors.Is(err, config.ErrNotPaired) {
		return err
	}
	if err == nil {
		if err := r.Credentials.Delete(profile.DeviceID); err != nil {
			return err
		}
	}
	if err := r.Profiles.Delete(); err != nil {
		return err
	}
	return r.writeJSON(map[string]any{"paired": false})
}

func (r *Runner) call(ctx context.Context, tool string, arguments map[string]any) error {
	client, err := r.loadClient()
	if err != nil {
		return err
	}
	return r.callClient(ctx, client, tool, arguments)
}

// callWrite 在所有 CLI 写命令前统一检查 Starcat 当前能力，避免不同命令返回不一致的提示。
func (r *Runner) callWrite(
	ctx context.Context,
	tool string,
	capability string,
	arguments map[string]any,
) error {
	client, err := r.writeClient(ctx, capability)
	if err != nil {
		return err
	}
	return r.callClient(ctx, client, tool, arguments)
}

func (r *Runner) callClient(ctx context.Context, client *mcp.Client, tool string, arguments map[string]any) error {
	value, err := client.CallTool(ctx, tool, arguments)
	if err != nil {
		return err
	}
	return r.writeJSON(value)
}

// writeClient 使用公开的 capabilities Tool 作为唯一权限事实源，不在 CLI 复制 App 设置状态。
func (r *Runner) writeClient(ctx context.Context, capability string) (*mcp.Client, error) {
	client, err := r.loadClient()
	if err != nil {
		return nil, err
	}
	value, err := client.CallTool(ctx, "starcat.get_capabilities", map[string]any{})
	if err != nil {
		return nil, err
	}
	capabilities, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("Starcat returned an invalid capabilities response")
	}
	enabled, ok := capabilities[capability].(bool)
	if !ok {
		return nil, fmt.Errorf("Starcat capabilities response is missing %q", capability)
	}
	if enabled {
		return client, nil
	}
	switch capability {
	case "local_writes":
		return nil, errors.New("local writes are disabled; open Starcat > Settings > MCP Service, enable Allow Local Writes, and try again")
	case "destructive_writes":
		return nil, errors.New("replace/delete writes are disabled; open Starcat > Settings > MCP Service, enable Allow Local Writes and Allow Replace/Delete Writes, then try again")
	default:
		return nil, fmt.Errorf("Starcat write capability %q is disabled", capability)
	}
}

func (r *Runner) loadClient() (*mcp.Client, error) {
	transport, err := r.loadTransport()
	if err != nil {
		return nil, err
	}
	return mcp.NewClient(transport), nil
}

func (r *Runner) loadTransport() (*mcp.HTTPTransport, error) {
	profile, err := r.Profiles.Load()
	if err != nil {
		return nil, err
	}
	token, err := r.Credentials.Get(profile.DeviceID)
	if err != nil {
		return nil, err
	}
	return mcp.NewHTTPTransport(profile, token)
}

func (r *Runner) writeJSON(value any) error {
	encoder := json.NewEncoder(r.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// readAllContext 让被 signal.NotifyContext 取消的 CLI 能立即退出。
// 终端读取本身没有跨平台的 context API，因此放到 goroutine 中；生产入口取消后会立刻结束进程，
// 缓冲 channel 则保证读取稍后结束时不会再次阻塞该 goroutine。
func readAllContext(ctx context.Context, reader io.Reader) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	results := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(reader)
		results <- result{data: data, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-results:
		return result.data, result.err
	}
}

// isInteractiveReader 区分真实终端与 Agent 常用的 pipe/file stdin，避免污染自动化 stderr。
func isInteractiveReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// interactiveNotePrompt 使用各平台真实的终端 EOF 快捷键，避免跨平台 CLI 给出错误操作指引。
func interactiveNotePrompt() string {
	if runtime.GOOS == "windows" {
		return "Reading note content from stdin. Press Ctrl+Z, then Enter to submit; press Ctrl+C to cancel."
	}
	return "Reading note content from stdin. Press Ctrl+D to submit or Ctrl+C to cancel."
}

func pairingInputPrompt() string {
	if runtime.GOOS == "windows" {
		return "Paste the one-time pairing URI, then press Ctrl+Z and Enter to submit. Press Ctrl+C to cancel."
	}
	return "Paste the one-time pairing URI, then press Ctrl+D to submit. Press Ctrl+C to cancel."
}

func repoSelector(value string) (map[string]any, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, errors.New("repository must use owner/name format")
	}
	return map[string]any{"owner": parts[0], "name": parts[1]}, nil
}

func parseFlags(args []string, valueFlags map[string]bool) ([]string, map[string][]string, error) {
	positionals := make([]string, 0, len(args))
	flags := make(map[string][]string)
	for index := 0; index < len(args); index++ {
		item := args[index]
		if !strings.HasPrefix(item, "--") {
			positionals = append(positionals, item)
			continue
		}
		name := strings.TrimPrefix(item, "--")
		if name == "json" { // 所有命令本来就只输出 JSON，保留该 flag 方便 Agent 明确意图。
			flags[name] = nil
			continue
		}
		if valueFlags != nil && valueFlags[name] {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, nil, fmt.Errorf("--%s requires a value", name)
			}
			index++
			flags[name] = []string{args[index]}
			continue
		}
		flags[name] = nil
	}
	return positionals, flags, nil
}

func hasFlag(flags map[string][]string, name string) bool {
	_, ok := flags[name]
	return ok
}

func valueFlag(flags map[string][]string, name, fallback string) string {
	values := flags[name]
	if len(values) == 1 {
		return values[0]
	}
	return fallback
}

func intFlag(flags map[string][]string, name string, fallback, minimum, maximum int) (int, error) {
	raw := valueFlag(flags, name, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("--%s must be between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

const usage = `starcat CLI

Pairing and diagnostics:
  starcat pair --stdin
  starcat unpair
  starcat doctor --json
  starcat capabilities --json
  starcat update

MCP server:
  starcat mcp

Read commands:
  starcat repo search <query> [--scope starred|knowledge|all] [--limit N] [--semantic]
  starcat repo context <owner/name>
  starcat repo readme <owner/name>
  starcat repo summary <owner/name> [--generate] [--allow-external-context]
  starcat tags list

Write commands (dry-run by default; --apply persists changes):
  starcat repo note set <owner/name> --stdin [--apply]
  starcat repo status set <owner/name> <unread|read|using> [--apply]
  starcat repo tags <add|remove|replace> <owner/name> <tag...> [--apply]
  starcat tag create <name> [--color HEX] [--icon SYMBOL] [--apply]`
