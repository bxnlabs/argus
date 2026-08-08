package provider

import (
	"fmt"
	"regexp"
)

// ProviderType identifies a supported AI provider.
type ProviderType string

const (
	ProviderClaude ProviderType = "claude"
	ProviderCodex  ProviderType = "codex"
	ProviderShell  ProviderType = "shell"
)

// Provider defines an AI coding agent CLI.
type Provider struct {
	ID               ProviderType
	Name             string
	CLI              string // command name (e.g. "claude")
	AutoApproveFlag  string // flag to skip permission prompts
	SupportsResume   bool
	ResumeArg        string
	ModelFlag        string
	SessionIDPattern string // regex with one capture group for extracting session ID from terminal output
}

// BuildCommandOptions are the options for building a CLI command string.
type BuildCommandOptions struct {
	AutoApprove bool
	SessionID   string // for resume
	Model       string
}

var providers = map[ProviderType]*Provider{}

func register(p *Provider) {
	providers[p.ID] = p
}

// Get returns a provider by ID.
func Get(id ProviderType) (*Provider, error) {
	p, ok := providers[id]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", id)
	}
	return p, nil
}

// All returns all registered providers.
func All() []*Provider {
	out := make([]*Provider, 0, len(providers))
	for _, id := range []ProviderType{ProviderClaude, ProviderCodex, ProviderShell} {
		if p, ok := providers[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

// IsValid checks if a provider ID is registered.
func IsValid(id ProviderType) bool {
	_, ok := providers[id]
	return ok
}

// BuildCommand constructs the full CLI command string for a provider.
func BuildCommand(id ProviderType, opts BuildCommandOptions) (string, error) {
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

// GetSessionIDPattern returns the session ID extraction regex for a provider.
// Returns empty string for providers that don't support resume.
func GetSessionIDPattern(id ProviderType) string {
	p, ok := providers[id]
	if !ok || !p.SupportsResume {
		return ""
	}
	return p.SessionIDPattern
}

// extractSessionID applies a provider's SessionIDPattern regex to terminal
// output and returns the last captured session ID, or empty string if no match.
func extractSessionID(pattern, output string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return ""
	}
	return last[1]
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
