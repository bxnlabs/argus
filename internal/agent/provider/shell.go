package provider

func init() {
	register(&Provider{
		ID:   AgentShell,
		Name: "Terminal",
		CLI:  "", // no command — just a shell
	})
}
