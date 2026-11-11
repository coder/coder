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
	// WorkloadIdentity is set when the command runs under a workspace
	// daemon. It is who Coder considers that daemon to be.
	WorkloadIdentity *codersdk.WorkloadIdentity `json:"workload_identity,omitempty" table:"-"`
}

func (r whoamiRow) String() string {
	out := fmt.Sprintf(
		Caret+"Coder is running at %s, You're authenticated as %s !\n",
		pretty.Sprint(cliui.DefaultStyles.Keyword, r.URL),
		pretty.Sprint(cliui.DefaultStyles.Keyword, r.Username),
	)
	if r.WorkloadIdentity != nil {
		out += formatWorkloadIdentity(*r.WorkloadIdentity)
	}
	return out
}

func (r *RootCmd) whoami() *serpent.Command {
	agentAuth := &AgentAuth{}
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
			ctx := inv.Context()
			// Under a workspace daemon, the process also has a workload
			// identity. It exists whether or not a user is logged in.
			var identity *codersdk.WorkloadIdentity
			if got, inWorkspace, err := r.workspaceIdentity(inv, agentAuth); inWorkspace && err == nil {
				identity = &got
			}
			noUser := func(err error) error {
				if identity == nil {
					return err
				}
				_, err = fmt.Fprintf(inv.Stdout, "No user is logged in here. This process acts as a workload.\n%s",
					formatWorkloadIdentity(*identity))
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return noUser(err)
			}
			// A workload can reach the user API within its scopes, but
			// nobody is logged in.
			if r.workloadSocket != "" && identity != nil {
				return noUser(nil)
			}

			// Fetch the user info
			resp, err := client.User(ctx, codersdk.Me)
			// Get Coder instance url
			clientURL := client.URL
			if err != nil {
				return noUser(err)
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
					WorkloadIdentity:    identity,
				},
			})
			if err != nil {
				return err
			}
			_, err = inv.Stdout.Write([]byte(out))
			return err
		},
	}
	agentAuth.AttachOptions(cmd, true)
	formatter.AttachOptions(&cmd.Options)
	return cmd
}
