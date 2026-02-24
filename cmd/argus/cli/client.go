package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"
)

// discoveryInfo mirrors agent.DiscoveryInfo without importing internal/.
type discoveryInfo struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
}

// discover reads the discovery file and validates PID liveness.
func discover(path string) (*discoveryInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Argus agent is not running.\nStart it with: argus --port 3000")
		}
		return nil, fmt.Errorf("read discovery file: %w", err)
	}

	var info discoveryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse discovery file: %w", err)
	}

	// Check if the PID is still alive.
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("Argus agent is not running (stale state detected, cleaning up).\nStart it with: argus --port 3000")
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("Argus agent is not running (stale state detected, cleaning up).\nStart it with: argus --port 3000")
	}

	return &info, nil
}

// apiClient makes HTTP requests to the Argus agent API.
type apiClient struct {
	baseURL string // e.g. "http://127.0.0.1:3000/agent"
	http    http.Client
}

// newClient reads the discovery file and returns an API client.
func newClient(discoveryPath string) (*apiClient, error) {
	info, err := discover(discoveryPath)
	if err != nil {
		return nil, err
	}

	c := &apiClient{
		baseURL: "http://" + info.Address + "/agent",
		http: http.Client{
			Timeout: 10 * time.Second,
		},
	}
	return c, nil
}

func (c *apiClient) get(path string) ([]byte, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *apiClient) post(path string, body io.Reader) ([]byte, int, error) {
	resp, err := c.http.Post(c.baseURL+path, "application/json", body)
	if err != nil {
		return nil, 0, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func (c *apiClient) patch(path string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPatch, c.baseURL+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func (c *apiClient) delete(path string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}
