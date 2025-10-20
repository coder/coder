package cli

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) tasksCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "task",
		Aliases: []string{"tasks"},
		Short:   "Experimental task commands.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.taskCreate(),
			r.taskDelete(),
			r.taskList(),
			r.taskLogs(),
			r.taskSend(),
			r.taskStatus(),
		},
	}
	return cmd
}

func splitTaskIdentifier(identifier string) (owner string, taskName string, err error) {
	parts := strings.Split(identifier, "/")

	switch len(parts) {
	case 1:
		owner = codersdk.Me
		taskName = parts[0]
	case 2:
		owner = parts[0]
		taskName = parts[1]
	default:
		return "", "", xerrors.Errorf("invalid task identifier: %q", identifier)
	}
	return owner, taskName, nil
}

// resolveTask fetches and returns a task by an identifier, which may be either
// a UUID, a bare name (for a task owned by the current user), or a "user/task"
// combination, where user is either a username or UUID.
//
// Since there is no TaskByOwnerAndName endpoint yet, this function uses the
// list endpoint with filtering when a name is provided.
func resolveTask(ctx context.Context, client *codersdk.Client, identifier string) (codersdk.Task, error) {
	exp := codersdk.NewExperimentalClient(client)

	identifier = strings.TrimSpace(identifier)

	// Try parsing as UUID first.
	if taskID, err := uuid.Parse(identifier); err == nil {
		return exp.TaskByID(ctx, taskID)
	}

	// Not a UUID, treat as identifier.
	owner, taskName, err := splitTaskIdentifier(identifier)
	if err != nil {
		return codersdk.Task{}, err
	}

	tasks, err := exp.Tasks(ctx, &codersdk.TasksFilter{
		Owner: owner,
	})
	if err != nil {
		return codersdk.Task{}, xerrors.Errorf("list tasks for owner %q: %w", owner, err)
	}

	if taskID, err := uuid.Parse(taskName); err == nil {
		// Find task by ID.
		for _, task := range tasks {
			if task.ID == taskID {
				return task, nil
			}
		}
	} else {
		// Find task by name.
		for _, task := range tasks {
			if task.Name == taskName {
				return task, nil
			}
		}
	}

	// Mimic resource not found from API.
	var notFoundErr error = &codersdk.Error{
		Response: codersdk.Response{Message: "Resource not found or you do not have access to this resource"},
	}
	return codersdk.Task{}, xerrors.Errorf("task %q not found for owner %q: %w", taskName, owner, notFoundErr)
}
