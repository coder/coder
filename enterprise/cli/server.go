package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/enterprise/coderd"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	var (
		auditLogging   bool
		scimAuthHeader string
	)
	cmd := agpl.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
		api, err := coderd.New(ctx, &coderd.Options{
			AuditLogging: auditLogging,
			ScimAPIKey:   []byte(scimAuthHeader),
			Options:      options,
		})
		if err != nil {
			return nil, err
		}
		return api.AGPL, nil
	})
	cliflag.BoolVarP(cmd.Flags(), &auditLogging, "audit-logging", "", "CODER_AUDIT_LOGGING", true,
		"Specifies whether audit logging is enabled.")
	cliflag.StringVarP(cmd.Flags(), &scimAuthHeader, "scim-auth-header", "", "CODER_SCIM_API_KEY", "", "Enables and sets the authentication header for the built-in SCIM server.")

	return cmd
}
