package cli

import (
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func users() *cobra.Command {
	cmd := &cobra.Command{
		Short:   "Create, remove, and list users",
		Use:     "users",
		Aliases: []string{"user"},
	}
	cmd.AddCommand(
		userCreate(),
		userList(),
		userSingle(),
		createUserStatusCommand(codersdk.UserStatusActive),
		createUserStatusCommand(codersdk.UserStatusSuspended),
	)
	return cmd
}

// displayUsers will return a table displaying all users passed in.
// filterColumns must be a subset of the user fields and will determine which
// columns to display
func displayUsers(filterColumns []string, users ...codersdk.User) string {
	tableWriter := cliui.Table()
	header := table.Row{"id", "username", "email", "created at", "status"}
	tableWriter.AppendHeader(header)
	tableWriter.SetColumnConfigs(cliui.FilterTableColumns(header, filterColumns))
	tableWriter.SortBy([]table.SortBy{{
		Name: "username",
	}})
	for _, user := range users {
		tableWriter.AppendRow(table.Row{
			user.ID.String(),
			user.Username,
			user.Email,
			user.CreatedAt.Format(time.Stamp),
			user.Status,
		})
	}
	return tableWriter.Render()
}
