package cli

import (
	"context"
	"io"
	"net/url"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/enterprise/coderd"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	dflags := deployment.Flags()
	cmd := agpl.Server(dflags, func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		if dflags.DerpServerRelayAddress.Value != "" {
			_, err := url.Parse(dflags.DerpServerRelayAddress.Value)
			if err != nil {
				return nil, nil, xerrors.Errorf("derp-server-relay-address must be a valid HTTP URL: %w", err)
			}
		}

		o := &coderd.Options{
			AuditLogging:           dflags.AuditLogging.Value,
			BrowserOnly:            dflags.BrowserOnly.Value,
			SCIMAPIKey:             []byte(dflags.SCIMAuthHeader.Value),
			UserWorkspaceQuota:     dflags.UserWorkspaceQuota.Value,
			RBAC:                   true,
			DERPServerRelayAddress: dflags.DerpServerRelayAddress.Value,
			DERPServerRegionID:     dflags.DerpServerRegionID.Value,

			Options: options,
		}
		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, nil, err
		}
		return api.AGPL, api, nil
	})

	deployment.AttachFlags(cmd.Flags(), dflags, true)
	return cmd
}
