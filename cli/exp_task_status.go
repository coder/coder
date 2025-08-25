package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskStatus() *serpent.Command {
	var (
		client = new(codersdk.Client)
		follow bool
	)
	cmd := &serpent.Command{
		Aliases: []string{"stat", "st"},
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()
			ec := codersdk.NewExperimentalClient(client)
			id := i.Args[0]

			// TODO: right now tasks are still "workspaces" under the hood.
			// We should update this once we have a proper task model.
			ws, err := namedWorkspace(ctx, client, id)
			if err != nil {
				return err
			}

			task, err := ec.TaskByID(ctx, ws.ID)
			if err != nil {
				return err
			}

			printTaskStatus(i.Stdout, task)
			if !follow {
				return nil
			}

			lastStatus := task.Status
			lastState := task.CurrentState
			t := time.NewTicker(1 * time.Second)
			defer t.Stop()
			// TODO: implement streaming updates instead of polling
			for range t.C {
				task, err := ec.TaskByID(ctx, ws.ID)
				if err != nil {
					return err
				}
				if lastStatus == task.Status {
					continue
				}
				if taskStatusEqual(lastState, task.CurrentState) {
					continue
				}
				printTaskStatus(i.Stdout, task)

				if task.Status == codersdk.WorkspaceStatusStopped {
					return nil
				}
				if task.CurrentState != nil && task.CurrentState.State != codersdk.TaskStateWorking {
					return nil
				}
				lastStatus = task.Status
				lastState = task.CurrentState
			}
			return nil
		},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{
			{
				Default:       "false",
				Description:   "Follow the task status output. This will stream updates to the terminal until the task completes.",
				Flag:          "follow",
				FlagShorthand: "f",
				Name:          "follow",
				Value:         serpent.BoolOf(&follow),
			},
		},
		Short: "Show the status of a task.",
		Use:   "status",
	}
	return cmd
}

func taskStatusEqual(s1, s2 *codersdk.TaskStateEntry) bool {
	if s1 == nil && s2 == nil {
		return true
	}
	if s1 == nil || s2 == nil {
		return false
	}
	return s1.State == s2.State
}

func printTaskStatus(w io.Writer, t codersdk.Task) {
	_, _ = fmt.Fprint(w, t.Status)
	_, _ = fmt.Fprint(w, ", ")
	if t.CurrentState != nil {
		_, _ = fmt.Fprint(w, t.CurrentState.State)
	} else {
		_, _ = fmt.Fprint(w, "unknown")
	}
	_, _ = fmt.Fprintln(w)
}
