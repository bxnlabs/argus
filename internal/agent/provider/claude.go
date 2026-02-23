package provider

func init() {
	register(&Provider{
		ID:              "claude",
		Name:            "Claude Code",
		CLI:             "claude",
		AutoApproveFlag: "--dangerously-skip-permissions",
		SupportsResume:  true,
		ResumeArg:      "--resume",
		ModelFlag:       "--model",
	})
}
