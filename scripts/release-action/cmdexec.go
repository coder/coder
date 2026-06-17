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
//
// Read-only methods (RunOutput, Run) execute unconditionally in both
// real and dry-run modes. Mutating methods (RunMutation,
// RunMutationStdout) are printed instead of executed in dry-run mode.
// Callers must choose the correct method at the call site so that
// dry-run behavior is explicit and reviewable.
type CommandExecutor interface {
	// RunOutput executes the named program with args and returns
	// trimmed stdout. Use for read-only commands.
	RunOutput(name string, args ...string) (string, error)

	// Run executes the named program with args, discarding output.
	// Use for read-only commands where only the exit code matters.
	Run(name string, args ...string) error

	// RunMutation executes a command that modifies remote state
	// (e.g. git push, git tag). In dry-run mode it prints instead
	// of executing.
	RunMutation(name string, args ...string) error

	// RunMutationStdout executes a mutating command, streaming
	// stdout and stderr to the provided writers. Stdin is set to
	// empty to prevent interactive prompts. In dry-run mode it
	// prints instead of executing.
	RunMutationStdout(stdout, stderr io.Writer, name string, args ...string) error
}

// realExecutor runs all commands for real via os/exec.
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

func (realExecutor) RunMutation(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

func (realExecutor) RunMutationStdout(stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = strings.NewReader("") // prevent interactive prompts
	return cmd.Run()
}

// dryRunExecutor delegates read-only commands to the real executor
// and prints mutating commands instead of executing them.
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

func (d *dryRunExecutor) RunMutation(name string, args ...string) error {
	_, _ = fmt.Fprintf(d.w, "[dry-run] would run: %s %s\n", name, shelljoin(args))
	return nil
}

func (d *dryRunExecutor) RunMutationStdout(_, _ io.Writer, name string, args ...string) error {
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
