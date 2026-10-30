package cmd

import (
	_ "embed"
	"fmt"
)

var (
	//go:embed completions/amber.bash
	bashCompletion string
	//go:embed completions/amber.fish
	fishCompletion string
	//go:embed completions/amber.zsh
	zshCompletion string
)

func (a App) Completion(shell string) error {
	var script string
	switch shell {
	case "bash":
		script = bashCompletion
	case "fish":
		script = fishCompletion
	case "zsh":
		script = zshCompletion
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, fish, zsh)", shell)
	}

	_, err := fmt.Fprint(a.stdout, script)
	return err
}
