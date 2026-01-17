//go:build linux

package nsjail

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"syscall"
)

type command struct {
	description string
	cmd         *exec.Cmd
	ambientCaps []uintptr

	// If ignoreErr isn't empty and this specific error occurs, suppress it (don’t log it, don’t return it).
	ignoreErr string
}

func newCommand(
	description string,
	cmd *exec.Cmd,
	ambientCaps []uintptr,
) *command {
	return newCommandWithIgnoreErr(description, cmd, ambientCaps, "")
}

func newCommandWithIgnoreErr(
	description string,
	cmd *exec.Cmd,
	ambientCaps []uintptr,
	ignoreErr string,
) *command {
	return &command{
		description: description,
		cmd:         cmd,
		ambientCaps: ambientCaps,
		ignoreErr:   ignoreErr,
	}
}

func (cmd *command) isIgnorableError(err string) bool {
	return cmd.ignoreErr != "" && strings.Contains(err, cmd.ignoreErr)
}

type commandRunner struct {
	commands []*command
}

func newCommandRunner(commands []*command) *commandRunner {
	return &commandRunner{
		commands: commands,
	}
}

func (r *commandRunner) run() error {
	for _, command := range r.commands {
		command.cmd.SysProcAttr = &syscall.SysProcAttr{
			AmbientCaps: command.ambientCaps,
		}

		output, err := command.cmd.CombinedOutput()
		if err != nil && !command.isIgnorableError(err.Error()) && !command.isIgnorableError(string(output)) {
			return fmt.Errorf("failed to %s: %v, output: %s", command.description, err, output)
		}
	}

	return nil
}

func (r *commandRunner) runIgnoreErrors() error {
	for _, command := range r.commands {
		command.cmd.SysProcAttr = &syscall.SysProcAttr{
			AmbientCaps: command.ambientCaps,
		}

		output, err := command.cmd.CombinedOutput()
		if err != nil && !command.isIgnorableError(err.Error()) && !command.isIgnorableError(string(output)) {
			log.Printf("err: %v", err)
			log.Printf("")

			log.Printf("failed to %s: %v, output: %s", command.description, err, output)
			continue
		}
	}

	return nil
}
