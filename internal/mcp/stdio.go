package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// BridgeStdio 将 Agent 的逐行 JSON-RPC stdio 消息原样桥接到 Starcat MCP HTTP。
// stdout 只允许出现 MCP 协议响应，诊断信息必须由上层写到 stderr。
func BridgeStdio(ctx context.Context, transport *HTTPTransport, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 8<<20)
	writer := bufio.NewWriter(output)
	defer writer.Flush()

	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			return fmt.Errorf("stdin contains an invalid JSON-RPC message")
		}
		status, body, err := transport.Send(ctx, line)
		if err != nil {
			return err
		}
		// MCP notification 通常返回 202 且没有 body，不能向 stdout 注入空响应。
		if status == http.StatusAccepted || len(body) == 0 {
			continue
		}
		if _, err := writer.Write(append(body, '\n')); err != nil {
			return fmt.Errorf("write MCP stdout: %w", err)
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	return scanner.Err()
}
