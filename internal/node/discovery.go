package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DiscoveryInfo is the content of ~/.argus/node.json.
type DiscoveryInfo struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
}

// DefaultDiscoveryPath returns ~/.argus/node.json.
func DefaultDiscoveryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".argus", "node.json"), nil
}

// WriteDiscoveryFile writes the node discovery file.
func WriteDiscoveryFile(path, address string) error {
	info := DiscoveryInfo{
		PID:     os.Getpid(),
		Address: address,
	}
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal discovery: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir discovery: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write discovery: %w", err)
	}
	return nil
}

// RemoveDiscoveryFile removes the discovery file. Errors are silently ignored
// (the file may have already been cleaned up).
func RemoveDiscoveryFile(path string) {
	os.Remove(path)
}
