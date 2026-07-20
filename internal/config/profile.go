// Package config 管理 Starcat CLI 的非敏感连接配置。
// 长期访问凭据必须交给 credential.Store，禁止写入 profile.json。
package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CurrentProtocolVersion = "1"

var ErrNotPaired = errors.New("Starcat CLI is not paired; run `starcat pair`")

// Profile 是 CLI 与一台 Starcat App 建立连接所需的非敏感资料。
type Profile struct {
	Endpoint          string    `json:"endpoint"`
	CertificateSHA256 string    `json:"certificate_sha256,omitempty"`
	DeviceID          string    `json:"device_id"`
	AppVersion        string    `json:"app_version"`
	ProtocolVersion   string    `json:"protocol_version"`
	PairedAt          time.Time `json:"paired_at"`
}

// Validate 确保 CLI 不会把凭据发送到明文远程地址，也拒绝协议版本漂移。
func (p Profile) Validate() error {
	if p.ProtocolVersion != CurrentProtocolVersion {
		return fmt.Errorf("CLI/App protocol mismatch: CLI=%s, Starcat=%s", CurrentProtocolVersion, p.ProtocolVersion)
	}
	u, err := url.Parse(p.Endpoint)
	if err != nil || u.Hostname() == "" || u.Path != "/mcp" {
		return errors.New("invalid Starcat endpoint: path must be /mcp")
	}
	switch u.Scheme {
	case "https":
		fingerprint := normalizeFingerprint(p.CertificateSHA256)
		if len(fingerprint) != 64 {
			return errors.New("remote HTTPS endpoint requires a valid SHA-256 certificate fingerprint")
		}
		if _, err := hex.DecodeString(fingerprint); err != nil {
			return errors.New("remote HTTPS endpoint certificate fingerprint is not valid hexadecimal")
		}
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return errors.New("plaintext HTTP is only allowed for loopback addresses")
		}
	default:
		return errors.New("Starcat endpoint must use http or https")
	}
	return nil
}

// Store 抽象 profile 持久化，便于命令测试替换为内存实现。
type Store interface {
	Load() (Profile, error)
	Save(Profile) error
	Delete() error
}

// FileStore 将非敏感资料保存到用户配置目录，并在 Unix 上限制为当前用户可读写。
type FileStore struct {
	Path string
}

func DefaultFileStore() (FileStore, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return FileStore{}, fmt.Errorf("locate user configuration directory: %w", err)
	}
	return FileStore{Path: filepath.Join(root, "starcat", "profile.json")}, nil
}

func (s FileStore) Load() (Profile, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Profile{}, ErrNotPaired
	}
	if err != nil {
		return Profile{}, fmt.Errorf("read connection profile: %w", err)
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf("decode connection profile: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (s FileStore) Save(profile Profile) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("encode connection profile: %w", err)
	}
	temporary := s.Path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write temporary connection profile: %w", err)
	}
	if err := os.Rename(temporary, s.Path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("save connection profile: %w", err)
	}
	return nil
}

func (s FileStore) Delete() error {
	err := os.Remove(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeFingerprint(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), ":", ""))
}
