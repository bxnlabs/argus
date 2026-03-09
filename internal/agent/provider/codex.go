package provider

func init() {
	register(&Provider{
		ID:              AgentCodex,
		Name:            "Codex",
		CLI:             "codex",
		AutoApproveFlag: "--approval-mode full-auto",
		SupportsResume:  true,
		ResumeArg:       "resume",
		ModelFlag:        "--model",
		SessionIDPattern: `codex resume ([0-9a-f-]+)`,
	})
}
