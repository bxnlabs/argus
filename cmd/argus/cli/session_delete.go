package cli

import "fmt"

func runDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("session name or ID required\n\nUsage: argus session rm <name-or-id>")
	}
	query := args[0]

	path, err := discoveryFilePath()
	if err != nil {
		return err
	}
	c, err := newClient(path)
	if err != nil {
		return err
	}

	session, err := fetchAndResolve(c, query)
	if err != nil {
		return err
	}

	if _, err := c.delete("/api/sessions/" + session.ID); err != nil {
		return err
	}

	fmt.Printf("Deleted session %q\n", session.Name)
	return nil
}
