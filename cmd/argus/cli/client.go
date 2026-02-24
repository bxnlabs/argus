package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

	// Check if the PID is still alive using kill(pid, 0).
	if err := syscall.Kill(info.PID, 0); err != nil {
		if errors.Is(err, syscall.EPERM) {
			// Process exists but is owned by another user — agent is running.
			return &info, nil
		}
		// ESRCH or other error: process is gone.
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

	// Validate the address is a loopback IP to prevent redirection
	// via a tampered discovery file.
	host, _, err := net.SplitHostPort(info.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid address in discovery file: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("refusing to connect to non-loopback address %q from discovery file", host)
	}

	c := &apiClient{
		baseURL: "http://" + info.Address + "/agent",
		http: http.Client{
			Timeout: 10 * time.Second,
		},
	}
	return c, nil
}

// checkStatus returns an error if status >= 400, extracting the JSON error
// message from the response body if available.
func checkStatus(body []byte, status int, op string) error {
	if status < 400 {
		return nil
	}
	var errResp struct {
		Error string `json:"error"`
	}
	json.Unmarshal(body, &errResp)
	if errResp.Error != "" {
		return fmt.Errorf("%s failed: %s", op, errResp.Error)
	}
	return fmt.Errorf("%s failed (HTTP %d)", op, status)
}

func (c *apiClient) get(path string) ([]byte, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("agent error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("agent error (HTTP %d)", resp.StatusCode)
	}
	return body, nil
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
