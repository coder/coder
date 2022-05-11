package cli

import (
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func userList() *cobra.Command {
	var (
		columns []string
	)
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			users, err := client.Users(cmd.Context(), codersdk.UsersRequest{})
			if err != nil {
				return err
			}

			tableWriter := cliui.Table()
			header := table.Row{"Username", "Email", "Created At"}
			tableWriter.AppendHeader(header)
			tableWriter.SetColumnConfigs(cliui.FilterTableColumns(header, columns))
			tableWriter.SortBy([]table.SortBy{{
				Name: "Username",
			}})
			for _, user := range users {
				tableWriter.AppendRow(table.Row{
					user.Username,
					user.Email,
					user.CreatedAt.Format(time.Stamp),
				})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), tableWriter.Render())
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&columns, "column", "c", nil,
		"Specify a column to filter in the table.")
	return cmd
}
