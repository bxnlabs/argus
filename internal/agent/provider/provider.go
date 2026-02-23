package provider

import "fmt"

// Provider defines an AI coding agent CLI.
type Provider struct {
	ID              string
	Name            string
	CLI             string // command name (e.g. "claude")
	AutoApproveFlag string // flag to skip permission prompts
	SupportsResume  bool
	ResumeArg      string
	ModelFlag       string
}

// BuildCommandOptions are the options for building a CLI command string.
type BuildCommandOptions struct {
	AutoApprove bool
	SessionID   string // for resume
	Model       string
}

var providers = map[string]*Provider{}

func register(p *Provider) {
	providers[p.ID] = p
}

// Get returns a provider by ID.
func Get(id string) (*Provider, error) {
	p, ok := providers[id]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", id)
	}
	return p, nil
}

// All returns all registered providers.
func All() []*Provider {
	out := make([]*Provider, 0, len(providers))
	for _, id := range []string{"claude", "codex", "gemini", "shell"} {
		if p, ok := providers[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

// IsValid checks if a provider ID is registered.
func IsValid(id string) bool {
	_, ok := providers[id]
	return ok
}

// BuildCommand constructs the full CLI command string for a provider.
func BuildCommand(id string, opts BuildCommandOptions) (string, error) {
	p, err := Get(id)
	if err != nil {
		return "", err
	}

	if p.CLI == "" {
		// Shell provider — no command
		return "", nil
	}

	cmd := p.CLI

	if opts.AutoApprove && p.AutoApproveFlag != "" {
		cmd += " " + p.AutoApproveFlag
	}

	if opts.SessionID != "" && p.SupportsResume && p.ResumeArg != "" {
		cmd += " " + p.ResumeArg + " " + shellEscape(opts.SessionID)
	}

	if opts.Model != "" && p.ModelFlag != "" {
		cmd += " " + p.ModelFlag + " " + shellEscape(opts.Model)
	}

	return cmd, nil
}

func shellEscape(s string) string {
	// Simple single-quote escaping
	result := "'"
	for _, c := range s {
		if c == '\'' {
			result += `'\''`
		} else {
			result += string(c)
		}
	}
	result += "'"
	return result
}
