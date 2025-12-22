package cli

import (
	"bufio"
	"errors"
	"os/exec"

	"cdr.dev/slog"
	"github.com/coder/serpent"
	"golang.org/x/xerrors"
)

func (r *RootCmd) acpShimCommand() *serpent.Command {
	return &serpent.Command{
		Use:        "acp-shim",
		Middleware: serpent.RequireRangeArgs(1, -1),
		Handler:    handleAcpShimCommand,
	}
}

func handleAcpShimCommand(inv *serpent.Invocation) error {
	if len(inv.Args) == 0 {
		return errors.New("no arguments provided") // should not be possible
	}
	cmd := inv.Args[0]
	args := inv.Args[1:]
	proc := exec.CommandContext(inv.Context(), cmd, args...)
	proc.Stderr = proc.Stdout

	var stdoutCh chan string

	stdin, err := proc.StdinPipe()
	if err != nil {
		return xerrors.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := proc.StdoutPipe()
	if err != nil {
		return xerrors.Errorf("create stdout pipe: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(inv.Stdin)
		for scanner.Scan() {
			if _, err := stdin.Write(scanner.Bytes()); err != nil {
				inv.Logger.Fatal(inv.Context(), "write to cmd stdin", slog.Error(err))
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case stdoutCh <- scanner.Text():
			case <-inv.Context().Done():
				return
			}
		}
	}()

	return proc.Wait()
}
