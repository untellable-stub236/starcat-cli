// Package updater implements release discovery, verified self-updates, and update notifications.
// GitHub Release assets are the only update source; downloaded archives must match checksums.txt.
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	defaultAPIURL    = "https://api.github.com/repos/starcat-app/starcat-cli/releases/latest"
	maxMetadataBytes = 2 << 20
	maxArchiveBytes  = 128 << 20
	maxBinaryBytes   = 64 << 20
)

var (
	ErrDevelopmentBuild = errors.New("self-update is unavailable for development builds; install a published release first")
	ErrHomebrewManaged  = errors.New("this installation is managed by Homebrew; run `brew update && brew upgrade starcat`")
)

// Asset is the minimal GitHub Release asset shape needed by the updater.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release contains a published version and its downloadable assets.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Result is written as JSON by `starcat update`.
type Result struct {
	Updated        bool   `json:"updated"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Executable     string `json:"executable,omitempty"`
}

// Client keeps network and platform dependencies replaceable for tests.
type Client struct {
	HTTPClient *http.Client
	APIURL     string
	GOOS       string
	GOARCH     string
	Executable func() (string, error)
}

// NewClient returns the production GitHub Release client.
func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		APIURL:     defaultAPIURL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Executable: os.Executable,
	}
}

// Latest fetches the latest stable GitHub Release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "starcat-cli")
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("check GitHub Releases: %w", err)
	}
	defer response.Body.Close()
	data, err := readLimited(response.Body, maxMetadataBytes)
	if err != nil {
		return Release{}, fmt.Errorf("read GitHub Release metadata: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub Releases returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	var release Release
	if err := json.Unmarshal(data, &release); err != nil {
		return Release{}, fmt.Errorf("decode GitHub Release metadata: %w", err)
	}
	if !semver.IsValid(release.TagName) {
		return Release{}, fmt.Errorf("latest GitHub Release has invalid version %q", release.TagName)
	}
	return release, nil
}

// Update downloads, verifies, and atomically installs the newest supported release.
func (c *Client) Update(ctx context.Context, currentVersion string) (Result, error) {
	if currentVersion == "dev" || !semver.IsValid(currentVersion) {
		return Result{}, ErrDevelopmentBuild
	}
	executable, err := c.Executable()
	if err != nil {
		return Result{}, fmt.Errorf("locate current executable: %w", err)
	}
	if IsHomebrewPath(executable) {
		return Result{}, ErrHomebrewManaged
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return Result{}, fmt.Errorf("resolve current executable: %w", err)
	}
	if IsHomebrewPath(executable) {
		return Result{}, ErrHomebrewManaged
	}

	release, err := c.Latest(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		Executable:     executable,
	}
	if semver.Compare(release.TagName, currentVersion) <= 0 {
		return result, nil
	}

	archiveName, binaryName, err := assetNames(release.TagName, c.GOOS, c.GOARCH)
	if err != nil {
		return Result{}, err
	}
	archiveAsset, ok := findAsset(release.Assets, archiveName)
	if !ok {
		return Result{}, fmt.Errorf("release %s does not include %s", release.TagName, archiveName)
	}
	checksumsAsset, ok := findAsset(release.Assets, "checksums.txt")
	if !ok {
		return Result{}, fmt.Errorf("release %s does not include checksums.txt", release.TagName)
	}

	archive, err := c.download(ctx, archiveAsset.BrowserDownloadURL, maxArchiveBytes)
	if err != nil {
		return Result{}, fmt.Errorf("download %s: %w", archiveName, err)
	}
	checksums, err := c.download(ctx, checksumsAsset.BrowserDownloadURL, maxMetadataBytes)
	if err != nil {
		return Result{}, fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyChecksum(archiveName, archive, string(checksums)); err != nil {
		return Result{}, err
	}
	binary, err := extractBinary(archiveName, binaryName, archive)
	if err != nil {
		return Result{}, err
	}
	if err := stageAndApply(binary, executable); err != nil {
		return Result{}, err
	}
	result.Updated = true
	return result, nil
}

// IsHomebrewPath prevents self-update from mutating package-manager-owned files.
func IsHomebrewPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/Cellar/starcat/") || strings.Contains(normalized, "/linuxbrew/.linuxbrew/Cellar/starcat/")
}

func (c *Client) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "starcat-cli")
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return readLimited(response.Body, limit)
}

func assetNames(version, goos, goarch string) (archive, binary string, err error) {
	supported := (goos == "darwin" || goos == "linux") && (goarch == "arm64" || goarch == "amd64")
	if goos == "windows" && goarch == "amd64" {
		return fmt.Sprintf("starcat_%s_windows_amd64.zip", version), "starcat.exe", nil
	}
	if !supported {
		return "", "", fmt.Errorf("unsupported update platform %s/%s", goos, goarch)
	}
	return fmt.Sprintf("starcat_%s_%s_%s.tar.gz", version, goos, goarch), "starcat", nil
}

func findAsset(assets []Asset, name string) (Asset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}

func verifyChecksum(name string, data []byte, manifest string) error {
	var expected string
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == name {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("checksums.txt does not contain a valid SHA-256 entry for %s", name)
	}
	digest := sha256.Sum256(data)
	actual := hex.EncodeToString(digest[:])
	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", name, expected, actual)
	}
	return nil
}

func extractBinary(archiveName, binaryName string, data []byte) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("open update zip: %w", err)
		}
		for _, file := range reader.File {
			if filepath.Base(file.Name) != binaryName || file.FileInfo().IsDir() {
				continue
			}
			if file.UncompressedSize64 > maxBinaryBytes {
				return nil, errors.New("updated binary exceeds the size limit")
			}
			stream, err := file.Open()
			if err != nil {
				return nil, err
			}
			binary, readErr := readLimited(stream, maxBinaryBytes)
			closeErr := stream.Close()
			if readErr != nil {
				return nil, readErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			return binary, nil
		}
	} else {
		gzipReader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("open update tar.gz: %w", err)
		}
		defer gzipReader.Close()
		tarReader := tar.NewReader(gzipReader)
		for {
			header, err := tarReader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read update tar.gz: %w", err)
			}
			if filepath.Base(header.Name) != binaryName || header.Typeflag != tar.TypeReg {
				continue
			}
			if header.Size > maxBinaryBytes {
				return nil, errors.New("updated binary exceeds the size limit")
			}
			return readLimited(tarReader, maxBinaryBytes)
		}
	}
	return nil, fmt.Errorf("update archive does not contain %s", binaryName)
}

func stageAndApply(binary []byte, executable string) error {
	info, err := os.Stat(executable)
	if err != nil {
		return fmt.Errorf("inspect current executable: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(executable), ".starcat-update-*")
	if err != nil {
		return fmt.Errorf("create update file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(binary); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write update file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close update file: %w", err)
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}
	if err := os.Chmod(temporaryPath, mode); err != nil {
		return fmt.Errorf("make update executable: %w", err)
	}
	if err := applyStaged(temporaryPath, executable); err != nil {
		return fmt.Errorf("replace current executable: %w", err)
	}
	return nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d-byte limit", limit)
	}
	return data, nil
}
