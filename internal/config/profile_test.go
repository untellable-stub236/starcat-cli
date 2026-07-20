package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestProfileValidateAllowsLoopbackHTTP(t *testing.T) {
	profile := Profile{
		Endpoint:        "http://127.0.0.1:5551/mcp",
		DeviceID:        "device-1",
		ProtocolVersion: CurrentProtocolVersion,
	}
	if err := profile.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProfileValidateRejectsRemoteHTTP(t *testing.T) {
	profile := Profile{
		Endpoint:        "http://192.168.1.10:5551/mcp",
		DeviceID:        "device-1",
		ProtocolVersion: CurrentProtocolVersion,
	}
	if err := profile.Validate(); err == nil {
		t.Fatal("Validate() should reject plaintext remote endpoint")
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "nested", "profile.json")}
	want := Profile{
		Endpoint:        "http://localhost:5551/mcp",
		DeviceID:        "device-1",
		AppVersion:      "1.0.0",
		ProtocolVersion: CurrentProtocolVersion,
		PairedAt:        time.Unix(1_700_000_000, 0).UTC(),
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !got.PairedAt.Equal(want.PairedAt) || got.Endpoint != want.Endpoint || got.DeviceID != want.DeviceID {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}
