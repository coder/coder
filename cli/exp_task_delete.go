package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskDelete() *serpent.Command {
	client := new(codersdk.Client)

	cmd := &serpent.Command{
		Use:   "delete <task> [<task> ...]",
		Short: "Delete experimental tasks",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, -1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			exp := codersdk.NewExperimentalClient(client)

			type toDelete struct {
				ID      uuid.UUID
				Owner   string
				Display string
			}

			var items []toDelete
			for _, identifier := range inv.Args {
				identifier = strings.TrimSpace(identifier)
				if identifier == "" {
					return xerrors.New("task identifier cannot be empty or whitespace")
				}

				// Check task identifier, try UUID first.
				if id, err := uuid.Parse(identifier); err == nil {
					task, err := exp.TaskByID(ctx, id)
					if err != nil {
						return xerrors.Errorf("resolve task %q: %w", identifier, err)
					}
					display := fmt.Sprintf("%s/%s", task.OwnerName, task.Name)
					items = append(items, toDelete{ID: id, Display: display, Owner: task.OwnerName})
					continue
				}

				// Non-UUID, treat as a workspace identifier (name or owner/name).
				ws, err := namedWorkspace(ctx, client, identifier)
				if err != nil {
					return xerrors.Errorf("resolve task %q: %w", identifier, err)
				}
				display := ws.FullName()
				items = append(items, toDelete{ID: ws.ID, Display: display, Owner: ws.OwnerName})
			}

			// Confirm deletion of the tasks.
			var displayList []string
			for _, it := range items {
				displayList = append(displayList, it.Display)
			}
			_, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Delete these tasks: %s?", pretty.Sprint(cliui.DefaultStyles.Code, strings.Join(displayList, ", "))),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			for _, item := range items {
				if err := exp.DeleteTask(ctx, item.Owner, item.ID); err != nil {
					return xerrors.Errorf("delete task %q: %w", item.Display, err)
				}
				_, _ = fmt.Fprintln(
					inv.Stdout, "Deleted task "+pretty.Sprint(cliui.DefaultStyles.Keyword, item.Display)+" at "+cliui.Timestamp(time.Now()),
				)
			}

			return nil
		},
	}

	return cmd
}
