package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runCreate(args []string) error {
	fs := flag.NewFlagSet("argus session create", flag.ExitOnError)
	name := fs.String("name", "", "Session name (required)")
	provider := fs.String("provider", "claude", "Agent type (claude, codex, gemini, shell)")
	model := fs.String("model", "", "Model override")
	dir := fs.String("dir", ".", "Working directory")
	autoApprove := fs.Bool("auto-approve", false, "Enable auto-approve")
	prompt := fs.String("prompt", "", "Initial prompt to send after creation")
	fs.Parse(args)

	if *name == "" {
		return fmt.Errorf("--name is required\n\nUsage: argus session create --name <name> [flags]")
	}

	// Resolve working directory to absolute path.
	wd, err := filepath.Abs(*dir)
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}

	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	reqBody := map[string]any{
		"name":              *name,
		"agent_type":        *provider,
		"working_directory": wd,
		"auto_approve":      *autoApprove,
	}
	if *model != "" {
		reqBody["model"] = *model
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	body, status, err := c.post("/api/sessions", bytes.NewReader(data))
	if err != nil {
		return err
	}

	if status >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Error != "" {
			return fmt.Errorf("create failed: %s", errResp.Error)
		}
		return fmt.Errorf("create failed (HTTP %d)", status)
	}

	var resp struct {
		Session sessionInfo `json:"session"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	s := resp.Session
	fmt.Fprintf(os.Stdout, "Created session %q (%s)\n  ID:  %s\n  Dir: %s\n", s.Name, s.AgentType, s.ID, s.WorkingDirectory)

	// If --prompt was provided, print a note (sending keys requires tmux attach or API).
	if *prompt != "" {
		fmt.Fprintf(os.Stdout, "\nTo send the initial prompt, attach to the session:\n  argus session attach %s\n", s.Name)
	}

	return nil
}
