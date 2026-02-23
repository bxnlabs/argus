package provider

func init() {
	register(&Provider{
		ID:              "gemini",
		Name:            "Gemini CLI",
		CLI:             "gemini",
		AutoApproveFlag: "--yolomode",
		SupportsResume:  true,
		ResumeArg:       "--resume",
		ModelFlag:       "-m",
	})
}
