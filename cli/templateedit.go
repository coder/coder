package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

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
		Use:   "edit <template>",
		Args:  cobra.ExactArgs(1),
		Short: "Edit the metadata of a template by name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			template, err := client.TemplateByName(cmd.Context(), organization.ID, args[0])
			if err != nil {
				return err
			}

			req := codersdk.UpdateTemplateMeta{
				Description:                template.Description,
				MaxTTLMillis:               template.MaxTTLMillis,
				MinAutostartIntervalMillis: template.MinAutostartIntervalMillis,
			}

			if description != "" {
				req.Description = description
			}
			if maxTTL != 0 {
				req.MaxTTLMillis = maxTTL.Milliseconds()
			}
			if minAutostartInterval != 0 {
				req.MinAutostartIntervalMillis = minAutostartInterval.Milliseconds()
			}

			_, err = client.UpdateTemplateMeta(cmd.Context(), template.ID, req)
			if err != nil {
				return err
			}
			_, _ = fmt.Printf("Updated template metadata!\n")
			return nil
		},
	}

	cmd.Flags().StringVarP(&description, "description", "", "", "Edit the template deescription")
	cmd.Flags().DurationVarP(&maxTTL, "max_ttl", "", 0, "Edit the template maximum time before shutdown")
	cmd.Flags().DurationVarP(&minAutostartInterval, "min_autostart_interval", "", 0, "Edit the template minimum autostart interval")
	cliui.AllowSkipPrompt(cmd)

	return cmd
}
