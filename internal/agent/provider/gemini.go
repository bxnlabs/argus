package provider

func init() {
	register(&Provider{
		ID:              AgentGemini,
		Name:            "Gemini CLI",
		CLI:             "gemini",
		AutoApproveFlag: "--yolomode",
		SupportsResume:  true,
		ResumeArg:       "--resume",
		ModelFlag:       "-m",
	})
}
