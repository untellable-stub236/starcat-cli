package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/starcat-app/starcat-cli/internal/config"
)

func TestClientInitializesAndCallsTool(t *testing.T) {
	var initialized bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/mcp" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		var message map[string]any
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Fatal(err)
		}
		switch message["method"] {
		case "initialize":
			writeResult(t, writer, message["id"], map[string]any{"protocolVersion": MCPProtocolVersion})
		case "notifications/initialized":
			initialized = true
			writer.WriteHeader(http.StatusAccepted)
		case "tools/call":
			writeResult(t, writer, message["id"], map[string]any{
				"structuredContent": map[string]any{"server_version": "test"},
				"isError":           false,
			})
		default:
			http.Error(writer, "unknown", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	profile := config.Profile{
		Endpoint:        server.URL + "/mcp",
		DeviceID:        "device-1",
		ProtocolVersion: config.CurrentProtocolVersion,
	}
	transport, err := NewHTTPTransport(profile, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	value, err := NewClient(transport).CallTool(context.Background(), "starcat.get_capabilities", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	object, ok := value.(map[string]any)
	if !ok || object["server_version"] != "test" {
		t.Fatalf("CallTool() = %#v", value)
	}
	if !initialized {
		t.Fatal("client did not send notifications/initialized")
	}
}

func writeResult(t *testing.T, writer http.ResponseWriter, id any, result map[string]any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
		t.Fatal(err)
	}
}
