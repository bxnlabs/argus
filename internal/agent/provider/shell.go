package provider

func init() {
	register(&Provider{
		ID:   "shell",
		Name: "Terminal",
		CLI:  "", // no command — just a shell
	})
}
