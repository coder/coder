package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// workspaceListRow is the type provided to the OutputFormatter. This is a bit
// dodgy but it's the only way to do complex display code for one format vs. the
// other.
type workspaceListRow struct {
	// For JSON format:
	codersdk.Workspace `table:"-"`

	// For table format:
	WorkspaceName string `json:"-" table:"workspace,default_sort"`
	Template      string `json:"-" table:"template"`
	Status        string `json:"-" table:"status"`
	Healthy       string `json:"-" table:"healthy"`
	LastBuilt     string `json:"-" table:"last built"`
	Outdated      bool   `json:"-" table:"outdated"`
	StartsAt      string `json:"-" table:"starts at"`
	StopsAfter    string `json:"-" table:"stops after"`
	DailyCost     string `json:"-" table:"daily cost"`
}

func workspaceListRowFromWorkspace(now time.Time, usersByID map[uuid.UUID]codersdk.User, workspace codersdk.Workspace) workspaceListRow {
	status := codersdk.WorkspaceDisplayStatus(workspace.LatestBuild.Job.Status, workspace.LatestBuild.Transition)

	lastBuilt := now.UTC().Sub(workspace.LatestBuild.Job.CreatedAt).Truncate(time.Second)
	autostartDisplay := "-"
	if !ptr.NilOrEmpty(workspace.AutostartSchedule) {
		if sched, err := cron.Weekly(*workspace.AutostartSchedule); err == nil {
			autostartDisplay = fmt.Sprintf("%s %s (%s)", sched.Time(), sched.DaysOfWeek(), sched.Location())
		}
	}

	autostopDisplay := "-"
	if !ptr.NilOrZero(workspace.TTLMillis) {
		dur := time.Duration(*workspace.TTLMillis) * time.Millisecond
		autostopDisplay = durationDisplay(dur)
		if !workspace.LatestBuild.Deadline.IsZero() && workspace.LatestBuild.Deadline.Time.After(now) && status == "Running" {
			remaining := time.Until(workspace.LatestBuild.Deadline.Time)
			autostopDisplay = fmt.Sprintf("%s (%s)", autostopDisplay, relative(remaining))
		}
	}

	healthy := ""
	if status == "Starting" || status == "Started" {
		healthy = strconv.FormatBool(workspace.Health.Healthy)
	}
	user := usersByID[workspace.OwnerID]
	return workspaceListRow{
		Workspace:     workspace,
		WorkspaceName: user.Username + "/" + workspace.Name,
		Template:      workspace.TemplateName,
		Status:        status,
		Healthy:       healthy,
		LastBuilt:     durationDisplay(lastBuilt),
		Outdated:      workspace.Outdated,
		StartsAt:      autostartDisplay,
		StopsAfter:    autostopDisplay,
		DailyCost:     strconv.Itoa(int(workspace.LatestBuild.DailyCost)),
	}
}

func (r *RootCmd) list() *clibase.Cmd {
	var (
		all               bool
		defaultQuery      = "owner:me"
		searchQuery       string
		displayWorkspaces []workspaceListRow
		formatter         = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]workspaceListRow{},
				[]string{
					"workspace",
					"template",
					"status",
					"healthy",
					"last built",
					"outdated",
					"starts at",
					"stops after",
				},
			),
			cliui.JSONFormat(),
		)
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "list",
		Short:       "List workspaces",
		Aliases:     []string{"ls"},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			filter := codersdk.WorkspaceFilter{
				FilterQuery: searchQuery,
			}
			if all && searchQuery == defaultQuery {
				filter.FilterQuery = ""
			}

			res, err := client.Workspaces(inv.Context(), filter)
			if err != nil {
				return err
			}
			if len(res.Workspaces) == 0 {
				pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Prompt, "No workspaces found! Create one:\n")
				_, _ = fmt.Fprintln(inv.Stderr)
				_, _ = fmt.Fprintln(inv.Stderr, "  "+pretty.Sprint(cliui.DefaultStyles.Code, "coder create <name>"))
				_, _ = fmt.Fprintln(inv.Stderr)
				return nil
			}

			userRes, err := client.Users(inv.Context(), codersdk.UsersRequest{})
			if err != nil {
				return err
			}

			usersByID := map[uuid.UUID]codersdk.User{}
			for _, user := range userRes.Users {
				usersByID[user.ID] = user
			}

			now := time.Now()
			displayWorkspaces = make([]workspaceListRow, len(res.Workspaces))
			for i, workspace := range res.Workspaces {
				displayWorkspaces[i] = workspaceListRowFromWorkspace(now, usersByID, workspace)
			}

			out, err := formatter.Format(inv.Context(), displayWorkspaces)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:          "all",
			FlagShorthand: "a",
			Description:   "Specifies whether all workspaces will be listed or not.",

			Value: clibase.BoolOf(&all),
		},
		{
			Flag:        "search",
			Description: "Search for a workspace with a query.",
			Default:     defaultQuery,
			Value:       clibase.StringOf(&searchQuery),
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
