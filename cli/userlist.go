package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/coder/coder/codersdk"
)

func userList() *cobra.Command {
	return &cobra.Command{
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
			sort.Slice(users, func(i, j int) bool {
				return users[i].Username < users[j].Username
			})

			tableWriter := table.NewWriter()
			tableWriter.SetStyle(table.StyleLight)
			tableWriter.Style().Options.SeparateColumns = false
			tableWriter.AppendHeader(table.Row{"Username", "Email", "Created At"})
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
}
