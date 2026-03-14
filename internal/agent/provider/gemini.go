package provider

func init() {
	register(&Provider{
		ID:               AgentGemini,
		Name:             "Gemini CLI",
		CLI:              "gemini",
		AutoApproveFlag:  "--yolo",
		SupportsResume:   true,
		ResumeArg:        "--resume",
		ModelFlag:        "-m",
		SessionIDPattern: `Session ID:[[:space:]]+([0-9a-f-]+)`,
	})
}
