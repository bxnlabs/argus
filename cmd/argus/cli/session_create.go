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
	fs := flag.NewFlagSet("argus session new", flag.ExitOnError)
	name := fs.String("name", "", "Session name (required)")
	provider := fs.String("provider", "claude", "Agent type (claude, codex, gemini, shell)")
	dir := fs.String("dir", ".", "Working directory")
	yolo := fs.Bool("yolo", false, "Enable auto-approve")
	fs.Parse(args)

	if *name == "" {
		return fmt.Errorf("--name is required\n\nUsage: argus session new --name <name> [flags]")
	}

	// Resolve working directory to absolute path.
	wd, err := filepath.Abs(*dir)
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}

	path, err := discoveryFilePath()
	if err != nil {
		return err
	}
	c, err := newClient(path)
	if err != nil {
		return err
	}

	reqBody := map[string]any{
		"name":              *name,
		"agent_type":        *provider,
		"working_directory": wd,
		"auto_approve":      *yolo,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	body, err := c.post("/api/sessions", bytes.NewReader(data))
	if err != nil {
		return err
	}

	var resp struct {
		Session sessionInfo `json:"session"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	s := resp.Session
	fmt.Fprintf(os.Stderr, "Created session %q (%s)\n", s.Name, s.AgentType)

	return attachTmux(s.TmuxName)
}
