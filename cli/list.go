package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

// workspaceListRow is the type provided to the OutputFormatter. This is a bit
// dodgy but it's the only way to do complex display code for one format vs. the
// other.
type workspaceListRow struct {
	// For JSON format:
	codersdk.Workspace `table:"-"`

	// For table format:
	Favorite       bool   `json:"-" table:"favorite"`
	WorkspaceName  string `json:"-" table:"workspace,default_sort"`
	Template       string `json:"-" table:"template"`
	Status         string `json:"-" table:"status"`
	Healthy        string `json:"-" table:"healthy"`
	LastBuilt      string `json:"-" table:"last built"`
	CurrentVersion string `json:"-" table:"current version"`
	Outdated       bool   `json:"-" table:"outdated"`
	StartsAt       string `json:"-" table:"starts at"`
	StartsNext     string `json:"-" table:"starts next"`
	StopsAfter     string `json:"-" table:"stops after"`
	StopsNext      string `json:"-" table:"stops next"`
	DailyCost      string `json:"-" table:"daily cost"`
}

func workspaceListRowFromWorkspace(now time.Time, workspace codersdk.Workspace) workspaceListRow {
	status := codersdk.WorkspaceDisplayStatus(workspace.LatestBuild.Job.Status, workspace.LatestBuild.Transition)

	lastBuilt := now.UTC().Sub(workspace.LatestBuild.Job.CreatedAt).Truncate(time.Second)
	schedRow := scheduleListRowFromWorkspace(now, workspace)

	healthy := ""
	if status == "Starting" || status == "Started" {
		healthy = strconv.FormatBool(workspace.Health.Healthy)
	}
	favIco := " "
	if workspace.Favorite {
		favIco = "★"
	}
	workspaceName := favIco + " " + workspace.OwnerName + "/" + workspace.Name
	return workspaceListRow{
		Favorite:       workspace.Favorite,
		Workspace:      workspace,
		WorkspaceName:  workspaceName,
		Template:       workspace.TemplateName,
		Status:         status,
		Healthy:        healthy,
		LastBuilt:      durationDisplay(lastBuilt),
		CurrentVersion: workspace.LatestBuild.TemplateVersionName,
		Outdated:       workspace.Outdated,
		StartsAt:       schedRow.StartsAt,
		StartsNext:     schedRow.StartsNext,
		StopsAfter:     schedRow.StopsAfter,
		StopsNext:      schedRow.StopsNext,
		DailyCost:      strconv.Itoa(int(workspace.LatestBuild.DailyCost)),
	}
}

func (r *RootCmd) list() *serpent.Command {
	var (
		filter    cliui.WorkspaceFilter
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]workspaceListRow{},
				[]string{
					"workspace",
					"template",
					"status",
					"healthy",
					"last built",
					"current version",
					"outdated",
					"starts at",
					"stops after",
				},
			),
			cliui.JSONFormat(),
		)
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "list",
		Short:       "List workspaces",
		Aliases:     []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			res, err := queryConvertWorkspaces(inv.Context(), client, filter.Filter(), workspaceListRowFromWorkspace)
			if err != nil {
				return err
			}

			if len(res) == 0 {
				pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Prompt, "No workspaces found! Create one:\n")
				_, _ = fmt.Fprintln(inv.Stderr)
				_, _ = fmt.Fprintln(inv.Stderr, "  "+pretty.Sprint(cliui.DefaultStyles.Code, "coder create <name>"))
				_, _ = fmt.Fprintln(inv.Stderr)
				return nil
			}

			out, err := formatter.Format(inv.Context(), res)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	filter.AttachOptions(&cmd.Options)
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// queryConvertWorkspaces is a helper function for converting
// codersdk.Workspaces to a different type.
// It's used by the list command to convert workspaces to
// workspaceListRow, and by the schedule command to
// convert workspaces to scheduleListRow.
func queryConvertWorkspaces[T any](ctx context.Context, client *codersdk.Client, filter codersdk.WorkspaceFilter, convertF func(time.Time, codersdk.Workspace) T) ([]T, error) {
	var empty []T
	workspaces, err := client.Workspaces(ctx, filter)
	if err != nil {
		return empty, xerrors.Errorf("query workspaces: %w", err)
	}
	converted := make([]T, len(workspaces.Workspaces))
	for i, workspace := range workspaces.Workspaces {
		converted[i] = convertF(time.Now(), workspace)
	}
	return converted, nil
}
