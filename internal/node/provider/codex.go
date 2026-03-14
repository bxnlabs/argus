package provider

func init() {
	register(&Provider{
		ID:               ProviderCodex,
		Name:             "Codex",
		CLI:              "codex",
		AutoApproveFlag:  "--full-auto",
		SupportsResume:   true,
		ResumeArg:        "resume",
		ModelFlag:        "--model",
		SessionIDPattern: `codex resume ([0-9a-f-]+)`,
	})
}
