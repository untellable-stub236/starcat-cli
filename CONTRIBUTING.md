# Contributing

Thank you for improving Starcat CLI.

## Before opening a pull request

1. Open an issue for behavior or protocol changes so the CLI and Starcat app contracts can be reviewed together.
2. Keep business logic in the Starcat app. The CLI should remain a thin MCP client and stdio bridge.
3. Keep all command-line help, prompts, and errors in English.
4. Never add plaintext credential fallbacks, endpoint bypasses, or write operations that skip Starcat permissions.
5. Add tests for changed behavior.

## Local checks

```bash
go mod verify
go test ./...
go test -race ./...
go vet ./...
bash -n scripts/*.sh
```

Do not include generated `bin/` or `dist/` artifacts in commits.

## Commit and pull request scope

Keep changes focused and explain security or compatibility implications. A pull request should describe the user-visible outcome, tests run, and any Starcat app version requirement.
