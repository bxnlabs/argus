package provider

import "fmt"

// AgentType identifies a supported agent provider.
type AgentType string

const (
	AgentClaude AgentType = "claude"
	AgentCodex  AgentType = "codex"
	AgentGemini AgentType = "gemini"
	AgentShell  AgentType = "shell"
)

// Provider defines an AI coding agent CLI.
type Provider struct {
	ID               AgentType
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

var providers = map[AgentType]*Provider{}

func register(p *Provider) {
	providers[p.ID] = p
}

// Get returns a provider by ID.
func Get(id AgentType) (*Provider, error) {
	p, ok := providers[id]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", id)
	}
	return p, nil
}

// All returns all registered providers.
func All() []*Provider {
	out := make([]*Provider, 0, len(providers))
	for _, id := range []AgentType{AgentClaude, AgentCodex, AgentGemini, AgentShell} {
		if p, ok := providers[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

// IsValid checks if a provider ID is registered.
func IsValid(id AgentType) bool {
	_, ok := providers[id]
	return ok
}

// BuildCommand constructs the full CLI command string for a provider.
func BuildCommand(id AgentType, opts BuildCommandOptions) (string, error) {
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
func GetSessionIDPattern(id AgentType) string {
	p, ok := providers[id]
	if !ok || !p.SupportsResume {
		return ""
	}
	return p.SessionIDPattern
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
