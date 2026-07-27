package main

import (
	"errors"
	"os/exec"
	"strings"
)

// gitOutput runs a read-only git command and returns trimmed stdout.
func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", exitErr
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitRun runs a git command with stdout/stderr connected to the
// terminal.
func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
