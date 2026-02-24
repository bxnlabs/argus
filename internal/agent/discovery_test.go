package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDiscoveryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")

	if err := WriteDiscoveryFile(path, "127.0.0.1:3000"); err != nil {
		t.Fatalf("WriteDiscoveryFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var info DiscoveryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if info.Address != "127.0.0.1:3000" {
		t.Errorf("address = %q, want %q", info.Address, "127.0.0.1:3000")
	}
	if info.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", info.PID, os.Getpid())
	}
}

func TestRemoveDiscoveryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")

	if err := WriteDiscoveryFile(path, "127.0.0.1:3000"); err != nil {
		t.Fatalf("WriteDiscoveryFile: %v", err)
	}

	RemoveDiscoveryFile(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists after RemoveDiscoveryFile")
	}
}

func TestRemoveDiscoveryFile_Missing(t *testing.T) {
	// Should not panic on missing file.
	RemoveDiscoveryFile("/tmp/nonexistent-argus-test-file.json")
}
