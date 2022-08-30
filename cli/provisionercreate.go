package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

func provisionerCreate() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a provisioner daemon instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			provisionerName := args[0]

			provisionerDaemon, err := client.CreateProvisionerDaemon(cmd.Context(), codersdk.CreateProvisionerDaemonRequest{
				Name: provisionerName,
			})
			if err != nil {
				return err
			}

			if provisionerDaemon.AuthToken == nil {
				return xerrors.New("provisioner daemon was created without an auth token")
			}
			tokenArg := provisionerDaemon.AuthToken.String()

			_, _ = fmt.Fprintln(cmd.OutOrStderr(), `A new provisioner daemon has been registered.

Start the provisioner daemon with the following command:

coder provisioners run --token `+tokenArg)

			return nil
		},
	}
	return cmd
}
