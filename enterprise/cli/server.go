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
		auditLogging bool
	)
	cmd := agpl.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
		api, err := coderd.New(ctx, &coderd.Options{
			AuditLogging: auditLogging,
			Options:      options,
		})
		if err != nil {
			return nil, err
		}
		return api.AGPL, nil
	})
	cliflag.BoolVarP(cmd.Flags(), &auditLogging, "audit-logging", "", "CODER_AUDIT_LOGGING", true,
		"Specifies whether audit logging is enabled.")

	return cmd
}
