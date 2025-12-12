package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskLogs() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]codersdk.TaskLogEntry{},
			[]string{
				"type",
				"content",
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "logs <task>",
		Short: "Show a task's logs",
		Long: FormatExamples(
			Example{
				Description: "Show logs for a given task.",
				Command:     "coder task logs task1",
			}),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			var (
				ctx        = inv.Context()
				identifier = inv.Args[0]
			)

			task, err := client.TaskByIdentifier(ctx, identifier)
			if err != nil {
				return xerrors.Errorf("resolve task %q: %w", identifier, err)
			}

			logs, err := client.TaskLogs(ctx, codersdk.Me, task.ID)
			if err != nil {
				return xerrors.Errorf("get task logs: %w", err)
			}

			out, err := formatter.Format(ctx, logs.Logs)
			if err != nil {
				return xerrors.Errorf("format task logs: %w", err)
			}

			if out == "" {
				cliui.Infof(inv.Stderr, "No task logs found.")
				return nil
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
