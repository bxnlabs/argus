package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"
)

// discoveryInfo mirrors node.DiscoveryInfo without importing internal/.
type discoveryInfo struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
}

// discover reads the discovery file and validates PID liveness.
func discover(path string) (*discoveryInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Argus node is not running.\nStart it with: argus")
		}
		return nil, fmt.Errorf("read discovery file: %w", err)
	}

	var info discoveryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse discovery file: %w", err)
	}

	// Reject invalid PIDs before the kill check. PID 0 would target the
	// entire process group, letting a crafted discovery file pass validation.
	if info.PID <= 0 {
		os.Remove(path)
		return nil, fmt.Errorf("Argus node is not running (invalid PID in state file, cleaning up).\nStart it with: argus")
	}

	// Check if the PID is still alive using kill(pid, 0).
	if err := syscall.Kill(info.PID, 0); err != nil {
		if errors.Is(err, syscall.EPERM) {
			// Process exists but is owned by another user — node is running.
			return &info, nil
		}
		// ESRCH or other error: process is gone.
		os.Remove(path)
		return nil, fmt.Errorf("Argus node is not running (stale state detected, cleaning up).\nStart it with: argus")
	}

	return &info, nil
}

// apiClient makes HTTP requests to the Argus node API.
type apiClient struct {
	baseURL string // e.g. "http://127.0.0.1:3000/api/node"
	http    http.Client
}

// newClient reads the discovery file and returns an API client.
func newClient(discoveryPath string) (*apiClient, error) {
	info, err := discover(discoveryPath)
	if err != nil {
		return nil, err
	}

	c := &apiClient{
		baseURL: "http://" + info.Address + "/api/node",
		http: http.Client{
			// Lifecycle mutations (create, delete, change-profile) run user
			// hooks that can each take up to 30s, so the client must wait well
			// past the default. A refused connection still fails immediately.
			Timeout: 120 * time.Second,
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

// readResponse reads the response body and checks the status code.
func readResponse(resp *http.Response, op string) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if err := checkStatus(body, resp.StatusCode, op); err != nil {
		return nil, err
	}
	return body, nil
}

func (c *apiClient) get(path string) ([]byte, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus node at %s.\nCheck if the node is running.", c.baseURL)
	}
	return readResponse(resp, "get")
}

func (c *apiClient) post(path string, body io.Reader, op string) ([]byte, error) {
	return c.postWith(c.http, path, body, op)
}

// profileStackClientTimeout bounds profile stack up/down from the client side.
// It sits just above the server's stackOpTimeout (20m) so the server's timeout
// stays the source of truth for the compose operation itself, while still
// capping the wait if the handler ever wedges outside that bounded path — the
// CLI should never hang indefinitely.
const profileStackClientTimeout = 25 * time.Minute

// postLongRunning issues a POST for server operations (profile stack up/down)
// whose duration is bounded server-side by stackOpTimeout and can far exceed the
// default client timeout — a cold `docker compose up --build --pull always`
// routinely runs for minutes. A dead node still fails fast because the dropped
// connection surfaces as a read error.
func (c *apiClient) postLongRunning(path string, body io.Reader, op string) ([]byte, error) {
	return c.postWith(http.Client{Timeout: profileStackClientTimeout}, path, body, op)
}

func (c *apiClient) postWith(client http.Client, path string, body io.Reader, op string) ([]byte, error) {
	resp, err := client.Post(c.baseURL+path, "application/json", body)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus node at %s.\nCheck if the node is running.", c.baseURL)
	}
	return readResponse(resp, op)
}

func (c *apiClient) patch(path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPatch, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus node at %s.\nCheck if the node is running.", c.baseURL)
	}
	return readResponse(resp, "update")
}

func (c *apiClient) put(path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPut, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus node at %s.\nCheck if the node is running.", c.baseURL)
	}
	return readResponse(resp, "update")
}

func (c *apiClient) delete(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus node at %s.\nCheck if the node is running.", c.baseURL)
	}
	return readResponse(resp, "delete")
}
