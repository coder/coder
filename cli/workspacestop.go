package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/database"
	"github.com/spf13/cobra"
)

func workspaceStop() *cobra.Command {
	return &cobra.Command{
		Use:  "stop <name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			workspaceHistory, err := client.WorkspaceHistory(cmd.Context(), "", args[0], "")
			if err != nil {
				return err
			}
			fmt.Printf("history: %+v\n", workspaceHistory)

			start := time.Now()
			history, err := client.CreateWorkspaceHistory(cmd.Context(), "", args[0], coderd.CreateWorkspaceHistoryRequest{
				ProjectVersionID: workspaceHistory.ProjectVersionID,
				Transition:       database.WorkspaceTransitionStop,
			})
			if err != nil {
				return err
			}
			fmt.Printf("History: %+v\n", history)

			logs, err := client.WorkspaceProvisionJobLogsAfter(cmd.Context(), organization.Name, history.ProvisionJobID, start)
			if err != nil {
				return err
			}
			for {
				log, ok := <-logs
				if !ok {
					return nil
				}
				fmt.Printf("Log: %s\n", log.Output)
			}

			return nil
		},
	}
}
