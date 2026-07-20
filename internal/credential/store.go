// Package credential 将设备凭据保存到操作系统安全存储。
// CLI 不提供明文文件降级，避免用户误以为凭据已经受系统保护。
package credential

import (
	"errors"
	"fmt"

	keyring "github.com/zalando/go-keyring"
)

const service = "com.starcat.cli"

var ErrNotFound = errors.New("未找到 Starcat 设备凭据，请重新配对")

// Store 是长期设备 token 的最小读写接口。
type Store interface {
	Get(deviceID string) (string, error)
	Set(deviceID, token string) error
	Delete(deviceID string) error
}

// KeyringStore 使用 macOS Keychain、Windows Credential Manager 或 Linux Secret Service。
type KeyringStore struct{}

func (KeyringStore) Get(deviceID string) (string, error) {
	value, err := keyring.Get(service, deviceID)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("读取系统安全存储：%w", err)
	}
	return value, nil
}

func (KeyringStore) Set(deviceID, token string) error {
	if err := keyring.Set(service, deviceID, token); err != nil {
		return fmt.Errorf("写入系统安全存储：%w", err)
	}
	return nil
}

func (KeyringStore) Delete(deviceID string) error {
	err := keyring.Delete(service, deviceID)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("删除系统安全存储凭据：%w", err)
	}
	return nil
}
