package main

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// CommandExecutor abstracts running CLI commands so that a dry-run
// implementation can print commands instead of executing them.
type CommandExecutor interface {
	// RunOutput executes the named program with args and returns
	// its trimmed stdout.
	RunOutput(name string, args ...string) (string, error)

	// Run executes the named program with args, discarding output.
	// Use this when only the exit code matters.
	Run(name string, args ...string) error

	// RunStdout executes the named program with args, streaming
	// stdout and stderr to the provided writers. Stdin is set to
	// empty to prevent interactive prompts.
	RunStdout(stdout, stderr io.Writer, name string, args ...string) error
}

// realExecutor runs commands for real via os/exec.
type realExecutor struct{}

func (realExecutor) RunOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
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

func (realExecutor) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

func (realExecutor) RunStdout(stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = strings.NewReader("") // prevent interactive prompts
	return cmd.Run()
}

// dryRunExecutor prints commands instead of executing them. Read-only
// commands (git fetch, git tag --list, etc.) are still executed so
// that version calculation and notes generation work correctly.
// Only mutating commands (gh release create) are printed.
type dryRunExecutor struct {
	w    io.Writer
	real realExecutor
}

func newDryRunExecutor(w io.Writer) *dryRunExecutor {
	return &dryRunExecutor{w: w}
}

func (d *dryRunExecutor) RunOutput(name string, args ...string) (string, error) {
	return d.real.RunOutput(name, args...)
}

func (d *dryRunExecutor) Run(name string, args ...string) error {
	return d.real.Run(name, args...)
}

// RunStdout is only used for mutating commands (gh release create),
// so in dry-run mode it prints the command instead of running it.
func (d *dryRunExecutor) RunStdout(stdout, _ io.Writer, name string, args ...string) error {
	_, _ = fmt.Fprintf(d.w, "[dry-run] would run: %s %s\n", name, shelljoin(args))
	return nil
}

// shelljoin produces a shell-safe representation of args for display.
func shelljoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\n\"'\\$") {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", "'\"'\"'") + "'"
			continue
		}
		quoted[i] = a
	}
	return strings.Join(quoted, " ")
}
