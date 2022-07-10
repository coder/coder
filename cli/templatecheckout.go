package cli

import (
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/provisionersdk"
)

func templateCheckout() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkout <template name> [destination]",
		Short: "Download the named template's contents into a subdirectory.",
		Long:  "Download the named template's contents and extract them into a subdirectory named according to the destination or <template name> if no destination is specified.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			templateName := args[0]
			var destination string
			if len(args) > 1 {
				destination = args[1]
			} else {
				destination = templateName
			}

			raw, err := fetchTemplateArchiveBytes(cmd, templateName)
			if err != nil {
				return err
			}

			// Stat the destination to ensure nothing exists already.
			stat, _ := os.Stat(destination)
			if stat != nil {
				return xerrors.Errorf("template file/directory already exists: %s", destination)
			}

			return provisionersdk.Untar(destination, raw)
		},
	}

	cliui.AllowSkipPrompt(cmd)

	return cmd
}
