package provider

func init() {
	register(&Provider{
		ID:               ProviderGemini,
		Name:             "Gemini CLI",
		CLI:              "gemini",
		AutoApproveFlag:  "--yolomode",
		SupportsResume:   true,
		ResumeArg:        "--resume",
		ModelFlag:        "-m",
		SessionIDPattern: `Session ID:[[:space:]]+([0-9a-f-]+)`,
	})
}
