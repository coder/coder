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
		name                         string
		displayName                  string
		description                  string
		icon                         string
		defaultTTL                   time.Duration
		allowUserCancelWorkspaceJobs bool
	)

	cmd := &cobra.Command{
		Use:   "edit <template> [flags]",
		Args:  cobra.ExactArgs(1),
		Short: "Edit the metadata of a template by name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}
			organization, err := CurrentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}
			template, err := client.TemplateByName(cmd.Context(), organization.ID, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace template: %w", err)
			}

			// NOTE: coderd will ignore empty fields.
			req := codersdk.UpdateTemplateMeta{
				Name:                         name,
				DisplayName:                  displayName,
				Description:                  description,
				Icon:                         icon,
				DefaultTTLMillis:             defaultTTL.Milliseconds(),
				AllowUserCancelWorkspaceJobs: allowUserCancelWorkspaceJobs,
			}

			_, err = client.UpdateTemplateMeta(cmd.Context(), template.ID, req)
			if err != nil {
				return xerrors.Errorf("update template metadata: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Updated template metadata at %s!\n", cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "", "", "Edit the template name")
	cmd.Flags().StringVarP(&displayName, "display-name", "", "", "Edit the template display name")
	cmd.Flags().StringVarP(&description, "description", "", "", "Edit the template description")
	cmd.Flags().StringVarP(&icon, "icon", "", "", "Edit the template icon path")
	cmd.Flags().DurationVarP(&defaultTTL, "default-ttl", "", 0, "Edit the template default time before shutdown - workspaces created from this template to this value.")
	cmd.Flags().BoolVarP(&allowUserCancelWorkspaceJobs, "allow-user-cancel-workspace-jobs", "", true, "Allow users to cancel in-progress workspace jobs.")
	cliui.AllowSkipPrompt(cmd)

	return cmd
}
