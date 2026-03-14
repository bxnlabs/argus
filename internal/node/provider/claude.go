package provider

func init() {
	register(&Provider{
		ID:               ProviderClaude,
		Name:             "Claude Code",
		CLI:              "claude",
		AutoApproveFlag:  "--dangerously-skip-permissions",
		SupportsResume:   true,
		ResumeArg:        "--resume",
		ModelFlag:        "--model",
		SessionIDPattern: `claude --resume ([0-9a-f-]+)`,
	})
}
