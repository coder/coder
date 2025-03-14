package cli

import (
	"errors"
	"fmt"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"

	"github.com/coder/serpent/completion"
)
func (*RootCmd) completion() *serpent.Command {
	var shellName string
	var printOutput bool

	shellOptions := completion.ShellOptions(&shellName)
	return &serpent.Command{
		Use:   "completion",
		Short: "Install or update shell completion scripts for the detected or chosen shell.",
		Options: []serpent.Option{
			{
				Flag:          "shell",
				FlagShorthand: "s",
				Description:   "The shell to install completion for.",
				Value:         shellOptions,
			},
			{
				Flag:          "print",
				Description:   "Print the completion script instead of installing it.",
				FlagShorthand: "p",
				Value: serpent.BoolOf(&printOutput),
			},
		},
		Handler: func(inv *serpent.Invocation) error {

			if shellName != "" {
				shell, err := completion.ShellByName(shellName, inv.Command.Parent.Name())
				if err != nil {
					return err
				}
				if printOutput {
					return shell.WriteCompletion(inv.Stdout)
				}
				return installCompletion(inv, shell)
			}
			shell, err := completion.DetectUserShell(inv.Command.Parent.Name())
			if err == nil {
				return installCompletion(inv, shell)
			}
			if !isTTYOut(inv) {
				return errors.New("could not detect the current shell, please specify one with --shell or run interactively")
			}
			// Silently continue to the shell selection if detecting failed in interactive mode
			choice, err := cliui.Select(inv, cliui.SelectOptions{
				Message: "Select a shell to install completion for:",
				Options: shellOptions.Choices,
			})
			if err != nil {
				return err
			}
			shellChoice, err := completion.ShellByName(choice, inv.Command.Parent.Name())
			if err != nil {
				return err
			}
			if printOutput {
				return shellChoice.WriteCompletion(inv.Stdout)
			}
			return installCompletion(inv, shellChoice)
		},
	}
}
func installCompletion(inv *serpent.Invocation, shell completion.Shell) error {
	path, err := shell.InstallPath()
	if err != nil {
		cliui.Error(inv.Stderr, fmt.Sprintf("Failed to determine completion path %v", err))
		return shell.WriteCompletion(inv.Stdout)

	}
	if !isTTYOut(inv) {
		return shell.WriteCompletion(inv.Stdout)
	}
	choice, err := cliui.Select(inv, cliui.SelectOptions{
		Options: []string{
			"Confirm",
			"Print to terminal",
		},
		Message:    fmt.Sprintf("Install completion for %s at %s?", shell.Name(), path),
		HideSearch: true,
	})
	if err != nil {
		return err
	}
	if choice == "Print to terminal" {
		return shell.WriteCompletion(inv.Stdout)
	}
	return completion.InstallShellCompletion(shell)
}
