package cli

import (
	"time"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

func users() *cobra.Command {
	cmd := &cobra.Command{
		Short: "Create, remove, and list users",
		Use:   "users",
	}
	cmd.AddCommand(
		userCreate(),
		userList(),
		userStatus(),
	)
	return cmd
}

func DisplayUsers(filterColumns []string, users ...codersdk.User) string {
	tableWriter := cliui.Table()
	header := table.Row{"ID", "Username", "Email", "Created At", "Status"}
	tableWriter.AppendHeader(header)
	tableWriter.SetColumnConfigs(cliui.FilterTableColumns(header, filterColumns))
	tableWriter.SortBy([]table.SortBy{{
		Name: "Username",
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
