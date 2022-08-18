package cli

import (
	"github.com/spf13/cobra"
)

func provisioners() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "provisioners",
		Short:   "Create, manage and run standalone provisioner daemons",
		Aliases: []string{"provisioner"},
	}
	cmd.AddCommand(
		provisionerRun(),
		provisionerCreate(),
	)

	return cmd
}
