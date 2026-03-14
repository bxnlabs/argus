package provider

func init() {
	register(&Provider{
		ID:   ProviderShell,
		Name: "Terminal",
		CLI:  "", // no command — just a shell
	})
}
