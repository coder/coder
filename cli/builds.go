package cli

import (
	"fmt"
	"strconv"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type workspaceBuildListRow struct {
	codersdk.WorkspaceBuild `table:"-"`

	BuildNumber string `json:"-" table:"build,default_sort"`
	BuildID     string `json:"-" table:"build id"`
	Status      string `json:"-" table:"status"`
	Reason      string `json:"-" table:"reason"`
	CreatedAt   string `json:"-" table:"created"`
	Duration    string `json:"-" table:"duration"`
}

func workspaceBuildListRowFromBuild(build codersdk.WorkspaceBuild) workspaceBuildListRow {
	status := codersdk.WorkspaceDisplayStatus(build.Job.Status, build.Transition)
	createdAt := build.CreatedAt.Format("2006-01-02 15:04:05")

	duration := ""
	if build.Job.CompletedAt != nil {
		duration = build.Job.CompletedAt.Sub(build.CreatedAt).Truncate(time.Second).String()
	}

	return workspaceBuildListRow{
		WorkspaceBuild: build,
		BuildNumber:    strconv.Itoa(int(build.BuildNumber)),
		BuildID:        build.ID.String(),
		Status:         status,
		Reason:         string(build.Reason),
		CreatedAt:      createdAt,
		Duration:       duration,
	}
}

func (r *RootCmd) builds() *serpent.Command {
	return &serpent.Command{
		Use:   "builds",
		Short: "Manage workspace builds",
		Children: []*serpent.Command{
			r.buildsList(),
		},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
}

func (r *RootCmd) buildsList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]workspaceBuildListRow{},
			[]string{"build", "build id", "status", "reason", "created", "duration"},
		),
		cliui.JSONFormat(),
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "list <workspace>",
		Short:       "List builds for a workspace",
		Aliases:     []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			builds, err := client.WorkspaceBuildsByWorkspaceID(inv.Context(), workspace.ID)
			if err != nil {
				return xerrors.Errorf("get workspace builds: %w", err)
			}

			rows := make([]workspaceBuildListRow, len(builds))
			for i, build := range builds {
				rows[i] = workspaceBuildListRowFromBuild(build)
			}

			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}
