package cli

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

type whoamiRow struct {
	URL                 string              `json:"url" table:"URL,default_sort"`
	Username            string              `json:"username" table:"Username"`
	UserID              string              `json:"user_id" table:"ID"`
	OrganizationIDs     string              `json:"-" table:"Orgs"`
	OrganizationIDsJSON []string            `json:"organization_ids" table:"-"`
	Roles               string              `json:"-" table:"Roles"`
	RolesJSON           map[string][]string `json:"roles" table:"-"`
}

func (r whoamiRow) String() string {
	return fmt.Sprintf(
		Caret+"Coder is running at %s, You're authenticated as %s !\n",
		pretty.Sprint(cliui.DefaultStyles.Keyword, r.URL),
		pretty.Sprint(cliui.DefaultStyles.Keyword, r.Username),
	)
}

func (r *RootCmd) whoami() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TextFormat(),
		cliui.JSONFormat(),
		cliui.TableFormat([]whoamiRow{}, []string{"url", "username", "id"}),
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "whoami",
		Short:       "Fetch authenticated user info for Coder deployment",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()
			// Fetch the user info
			resp, err := client.User(ctx, codersdk.Me)
			// Get Coder instance url
			clientURL := client.URL
			if err != nil {
				return err
			}

			orgIDs := make([]string, 0, len(resp.OrganizationIDs))
			for _, orgID := range resp.OrganizationIDs {
				orgIDs = append(orgIDs, orgID.String())
			}

			roles := make([]string, 0, len(resp.Roles))
			jsonRoles := make(map[string][]string)
			for _, role := range resp.Roles {
				if role.OrganizationID == "" {
					role.OrganizationID = "*"
				}
				roles = append(roles, fmt.Sprintf("%s:%s", role.OrganizationID, role.DisplayName))
				jsonRoles[role.OrganizationID] = append(jsonRoles[role.OrganizationID], role.DisplayName)
			}
			out, err := formatter.Format(ctx, []whoamiRow{
				{
					URL:                 clientURL.String(),
					Username:            resp.Username,
					UserID:              resp.ID.String(),
					OrganizationIDs:     strings.Join(orgIDs, ","),
					OrganizationIDsJSON: orgIDs,
					Roles:               strings.Join(roles, ","),
					RolesJSON:           jsonRoles,
				},
			})
			if err != nil {
				return err
			}
			_, err = inv.Stdout.Write([]byte(out))
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}
