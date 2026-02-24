package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func runRename(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("session name/ID and new name required\n\nUsage: argus session rename <name-or-id> <new-name>")
	}
	query := args[0]
	newName := args[1]

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

	reqBody, _ := json.Marshal(map[string]string{"name": newName})
	_, status, err := c.patch("/api/sessions/"+session.ID, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("rename failed (HTTP %d)", status)
	}

	fmt.Printf("Renamed session %q → %q\n", session.Name, newName)
	return nil
}
