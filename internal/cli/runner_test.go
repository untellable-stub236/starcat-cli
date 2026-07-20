package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dong4j/starcat-cli/internal/config"
	"github.com/dong4j/starcat-cli/internal/updater"
)

func TestRepoNoteRejectsDisabledWritesBeforeReadingStdin(t *testing.T) {
	stdin := &recordingReader{}
	runner, calls, closeServer := newWriteRunner(t, stdin, false)
	defer closeServer()

	err := runner.Run(context.Background(), []string{"repo", "note", "set", "toptal/gitignore.io", "--stdin"})
	if err == nil || !strings.Contains(err.Error(), "enable Allow Local Writes") {
		t.Fatalf("Run() error = %v, want local-write guidance", err)
	}
	if stdin.read {
		t.Fatal("stdin was read before local-write capability was checked")
	}
	if calls.upsertNote != 0 {
		t.Fatalf("upsert_repo_note calls = %d, want 0", calls.upsertNote)
	}
}

func TestRepoTagReplaceExplainsDestructiveWriteSetting(t *testing.T) {
	runner, calls, closeServer := newWriteRunner(t, strings.NewReader(""), true)
	defer closeServer()

	err := runner.Run(context.Background(), []string{"repo", "tags", "replace", "toptal/gitignore.io", "agent"})
	if err == nil || !strings.Contains(err.Error(), "enable Allow Local Writes and Allow Replace/Delete Writes") {
		t.Fatalf("Run() error = %v, want destructive-write guidance", err)
	}
	if calls.upsertNote != 0 {
		t.Fatalf("unexpected write tool calls = %d", calls.upsertNote)
	}
}

func TestPairOnlyAcceptsURIFromStdin(t *testing.T) {
	runner := &Runner{Stdin: strings.NewReader("invalid"), Stdout: io.Discard, Stderr: io.Discard}
	if err := runner.Run(context.Background(), []string{"pair", "starcat-pair://secret"}); err == nil || err.Error() != "Usage: starcat pair --stdin" {
		t.Fatalf("pair argument error = %v", err)
	}
	if err := runner.Run(context.Background(), []string{"pair", "--stdin"}); err == nil || !strings.Contains(err.Error(), "invalid pairing URI") {
		t.Fatalf("pair stdin error = %v", err)
	}
}

func TestUpdateWritesMachineReadableResult(t *testing.T) {
	var stdout bytes.Buffer
	runner := &Runner{
		Stdout: &stdout,
		RunUpdate: func(context.Context, string) (updater.Result, error) {
			return updater.Result{Updated: true, CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0"}, nil
		},
	}
	if err := runner.Run(context.Background(), []string{"update"}); err != nil {
		t.Fatalf("Run(update) error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"latest_version": "v1.1.0"`) {
		t.Fatalf("update stdout = %q", stdout.String())
	}
}

func TestRepoNotePipeInputDoesNotPrintInteractivePrompt(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner, calls, closeServer := newWriteRunner(t, strings.NewReader("pipeline note"), true)
	defer closeServer()
	runner.Stdout = &stdout
	runner.Stderr = &stderr

	if err := runner.Run(context.Background(), []string{"repo", "note", "set", "toptal/gitignore.io", "--stdin", "--apply"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no prompt for pipe input", stderr.String())
	}
	if calls.noteContent != "pipeline note" || calls.noteDryRun {
		t.Fatalf("note arguments = content %q, dry_run %v", calls.noteContent, calls.noteDryRun)
	}
	if !strings.Contains(stdout.String(), `"changed": true`) {
		t.Fatalf("stdout = %q, want JSON result", stdout.String())
	}
}

func TestRepoNoteInteractiveInputPrintsEOFAndCancelGuidance(t *testing.T) {
	var stderr bytes.Buffer
	runner, _, closeServer := newWriteRunner(t, strings.NewReader("interactive note"), true)
	defer closeServer()
	runner.Stderr = &stderr
	runner.stdinInteractive = true

	if err := runner.Run(context.Background(), []string{"repo", "note", "set", "toptal/gitignore.io", "--stdin"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "Ctrl+D") || !strings.Contains(stderr.String(), "Ctrl+C") {
		t.Fatalf("stderr = %q, want EOF and cancellation guidance", stderr.String())
	}
}

func TestRepoNoteStdinReadStopsWhenContextIsCanceled(t *testing.T) {
	stdin := newBlockingReader()
	runner, _, closeServer := newWriteRunner(t, stdin, true)
	defer closeServer()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- runner.Run(ctx, []string{"repo", "note", "set", "toptal/gitignore.io", "--stdin"})
	}()

	select {
	case <-stdin.started:
	case <-time.After(time.Second):
		t.Fatal("stdin read did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() remained blocked after context cancellation")
	}
	close(stdin.release)
}

type writeCalls struct {
	upsertNote  int
	noteContent string
	noteDryRun  bool
}

func newWriteRunner(t *testing.T, stdin io.Reader, localWrites bool) (*Runner, *writeCalls, func()) {
	t.Helper()
	calls := &writeCalls{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var message map[string]any
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		switch message["method"] {
		case "initialize":
			writeMCPResult(writer, message["id"], map[string]any{"protocolVersion": "2025-03-26"})
		case "notifications/initialized":
			writer.WriteHeader(http.StatusAccepted)
		case "tools/call":
			params, _ := message["params"].(map[string]any)
			tool, _ := params["name"].(string)
			arguments, _ := params["arguments"].(map[string]any)
			switch tool {
			case "starcat.get_capabilities":
				writeMCPToolResult(writer, message["id"], map[string]any{
					"local_writes":       localWrites,
					"destructive_writes": false,
				})
			case "starcat.upsert_repo_note":
				calls.upsertNote++
				calls.noteContent, _ = arguments["content"].(string)
				calls.noteDryRun, _ = arguments["dry_run"].(bool)
				writeMCPToolResult(writer, message["id"], map[string]any{"changed": !calls.noteDryRun})
			default:
				http.Error(writer, "unexpected tool: "+tool, http.StatusBadRequest)
			}
		default:
			http.Error(writer, "unexpected MCP method", http.StatusBadRequest)
		}
	}))

	profile := config.Profile{
		Endpoint:        server.URL + "/mcp",
		DeviceID:        "device-1",
		ProtocolVersion: config.CurrentProtocolVersion,
	}
	runner := &Runner{
		Profiles:    staticProfileStore{profile: profile},
		Credentials: staticCredentialStore{token: "test-token"},
		Stdin:       stdin,
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	}
	return runner, calls, server.Close
}

func writeMCPResult(writer http.ResponseWriter, id any, result map[string]any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeMCPToolResult(writer http.ResponseWriter, id any, structured map[string]any) {
	writeMCPResult(writer, id, map[string]any{
		"structuredContent": structured,
		"isError":           false,
	})
}

type staticProfileStore struct {
	profile config.Profile
}

func (s staticProfileStore) Load() (config.Profile, error) { return s.profile, nil }
func (s staticProfileStore) Save(config.Profile) error     { return nil }
func (s staticProfileStore) Delete() error                 { return nil }

type staticCredentialStore struct {
	token string
}

func (s staticCredentialStore) Get(string) (string, error) { return s.token, nil }
func (s staticCredentialStore) Set(string, string) error   { return nil }
func (s staticCredentialStore) Delete(string) error        { return nil }

type recordingReader struct {
	read bool
}

func (r *recordingReader) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("stdin should not be read")
}

type blockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingReader() *blockingReader {
	return &blockingReader{started: make(chan struct{}), release: make(chan struct{})}
}

func (r *blockingReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return 0, io.EOF
}
