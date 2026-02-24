package cli

import (
	"encoding/json"
	"fmt"
)

func runDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("session name or ID required\n\nUsage: argus session delete <name-or-id>")
	}
	query := args[0]

	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	// Fetch all sessions to resolve the query.
	body, err := c.get("/api/sessions")
	if err != nil {
		return err
	}

	var resp struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	session, err := resolveSession(resp.Sessions, query)
	if err != nil {
		return err
	}

	body2, status, err := c.delete("/api/sessions/" + session.ID)
	if err != nil {
		return err
	}
	if status >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body2, &errResp)
		if errResp.Error != "" {
			return fmt.Errorf("delete failed: %s", errResp.Error)
		}
		return fmt.Errorf("delete failed (HTTP %d)", status)
	}

	fmt.Printf("Deleted session %q\n", session.Name)
	return nil
}
