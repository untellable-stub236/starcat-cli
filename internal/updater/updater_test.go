package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateDownloadsVerifiesAndReplacesExecutable(t *testing.T) {
	newBinary := []byte("new starcat binary")
	archive := tarGzip(t, "starcat", newBinary)
	digest := sha256.Sum256(archive)
	archiveName := "starcat_v1.1.0_darwin_arm64.tar.gz"
	server := releaseServer(t, "v1.1.0", archiveName, archive, hex.EncodeToString(digest[:])+"  "+archiveName+"\n")
	defer server.Close()

	executable := filepath.Join(t.TempDir(), "starcat")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &Client{
		HTTPClient: server.Client(),
		APIURL:     server.URL + "/latest",
		GOOS:       "darwin",
		GOARCH:     "arm64",
		Executable: func() (string, error) { return executable, nil },
	}
	result, err := client.Update(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Updated || result.LatestVersion != "v1.1.0" {
		t.Fatalf("Update() = %#v", result)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Fatalf("installed binary = %q", got)
	}
}

func TestUpdateRejectsChecksumMismatchWithoutReplacingExecutable(t *testing.T) {
	archiveName := "starcat_v1.1.0_linux_amd64.tar.gz"
	archive := tarGzip(t, "starcat", []byte("untrusted"))
	server := releaseServer(t, "v1.1.0", archiveName, archive, strings.Repeat("0", 64)+"  "+archiveName+"\n")
	defer server.Close()

	executable := filepath.Join(t.TempDir(), "starcat")
	if err := os.WriteFile(executable, []byte("trusted old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &Client{
		HTTPClient: server.Client(),
		APIURL:     server.URL + "/latest",
		GOOS:       "linux",
		GOARCH:     "amd64",
		Executable: func() (string, error) { return executable, nil },
	}
	_, err := client.Update(context.Background(), "v1.0.0")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Update() error = %v", err)
	}
	got, readErr := os.ReadFile(executable)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "trusted old binary" {
		t.Fatalf("executable changed after failed verification: %q", got)
	}
}

func TestUpdateRejectsDevelopmentAndHomebrewBuilds(t *testing.T) {
	client := NewClient()
	if _, err := client.Update(context.Background(), "dev"); !errors.Is(err, ErrDevelopmentBuild) {
		t.Fatalf("Update(dev) error = %v", err)
	}
	client.Executable = func() (string, error) { return "/opt/homebrew/Cellar/starcat/1.0.0/bin/starcat", nil }
	if _, err := client.Update(context.Background(), "v1.0.0"); !errors.Is(err, ErrHomebrewManaged) {
		t.Fatalf("Update(Homebrew) error = %v", err)
	}
}

func TestAssetNamesCoverReleaseMatrix(t *testing.T) {
	tests := []struct {
		goos, goarch, archive, binary string
	}{
		{"darwin", "arm64", "starcat_v1.0.0_darwin_arm64.tar.gz", "starcat"},
		{"linux", "amd64", "starcat_v1.0.0_linux_amd64.tar.gz", "starcat"},
		{"windows", "amd64", "starcat_v1.0.0_windows_amd64.zip", "starcat.exe"},
	}
	for _, test := range tests {
		archive, binary, err := assetNames("v1.0.0", test.goos, test.goarch)
		if err != nil || archive != test.archive || binary != test.binary {
			t.Fatalf("assetNames(%s/%s) = %q, %q, %v", test.goos, test.goarch, archive, binary, err)
		}
	}
	if _, _, err := assetNames("v1.0.0", "freebsd", "amd64"); err == nil {
		t.Fatal("assetNames() should reject unsupported platforms")
	}
}

func TestWriteNotificationUsesStderrFriendlyText(t *testing.T) {
	var output bytes.Buffer
	WriteNotification(&output, "v1.2.0", "v1.1.0", "starcat update")
	if got := output.String(); !strings.Contains(got, "v1.2.0") || !strings.Contains(got, "starcat update") {
		t.Fatalf("notification = %q", got)
	}
}

func releaseServer(t *testing.T, version, archiveName string, archive []byte, checksums string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			_ = json.NewEncoder(writer).Encode(Release{
				TagName: version,
				HTMLURL: server.URL + "/release",
				Assets: []Asset{
					{Name: archiveName, BrowserDownloadURL: server.URL + "/archive"},
					{Name: "checksums.txt", BrowserDownloadURL: server.URL + "/checksums"},
				},
			})
		case "/archive":
			_, _ = writer.Write(archive)
		case "/checksums":
			_, _ = writer.Write([]byte(checksums))
		default:
			http.NotFound(writer, request)
		}
	}))
	return server
}

func tarGzip(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
