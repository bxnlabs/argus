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

	path, err := discoveryFilePath()
	if err != nil {
		return err
	}
	c, err := newClient(path)
	if err != nil {
		return err
	}

	session, err := fetchAndResolve(c, query)
	if err != nil {
		return err
	}

	reqBody, err := json.Marshal(map[string]string{"name": newName})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	respBody, status, err := c.patch("/api/sessions/"+session.ID, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if err := checkStatus(respBody, status, "rename"); err != nil {
		return err
	}

	fmt.Printf("Renamed session %q → %q\n", session.Name, newName)
	return nil
}
