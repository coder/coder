package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templateEdit() *cobra.Command {
	var (
		description          string
		maxTTL               time.Duration
		minAutostartInterval time.Duration
	)

	cmd := &cobra.Command{
		Use:   "edit <template> [flags]",
		Args:  cobra.ExactArgs(1),
		Short: "Edit the metadata of a template by name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}
			template, err := client.TemplateByName(cmd.Context(), organization.ID, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace template: %w", err)
			}

			// NOTE: coderd will ignore empty fields.
			req := codersdk.UpdateTemplateMeta{
				Description:                description,
				MaxTTLMillis:               maxTTL.Milliseconds(),
				MinAutostartIntervalMillis: minAutostartInterval.Milliseconds(),
			}

			_, err = client.UpdateTemplateMeta(cmd.Context(), template.ID, req)
			if err != nil {
				return xerrors.Errorf("update template metadata: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Updated template metadata at %s!\n", cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}

	cmd.Flags().StringVarP(&description, "description", "", "", "Edit the template description")
	cmd.Flags().DurationVarP(&maxTTL, "max_ttl", "", 0, "Edit the template maximum time before shutdown")
	cmd.Flags().DurationVarP(&minAutostartInterval, "min_autostart_interval", "", 0, "Edit the template minimum autostart interval")
	cliui.AllowSkipPrompt(cmd)

	return cmd
}
