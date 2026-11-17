//go:build !slim

package cli

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/serpent"
)

// setupTokenMinLength is the shortest setup token an operator may
// supply. 24 letters and digits is about 142 bits.
const setupTokenMinLength = 24

// resolveSetupToken decides whether this start requires a setup token
// and prints what the operator needs. It returns "" when no token is
// required.
//
// revive:disable-next-line:flag-parameter // The caller knows whether it opened a tunnel, and this function owns the decision.
func resolveSetupToken(
	ctx context.Context,
	inv *serpent.Invocation,
	db database.Store,
	supplied string,
	onTunnel bool,
) (string, error) {
	if !onTunnel {
		// A silent no-op would let an operator believe the first user
		// is protected when it is not.
		if supplied != "" {
			cliui.Warnf(inv.Stderr, "--setup-token is ignored because an access URL is set. "+
				"A setup token is required only on a try.coder.app tunnel. "+
				"To create the first user before the server can be reached, run coder server create-admin-user.")
		}
		return "", nil
	}

	//nolint:gocritic // User count requires system privileges
	userCount, err := db.GetUserCount(dbauthz.AsSystemRestricted(ctx), false)
	if err != nil {
		return "", xerrors.Errorf("get user count: %w", err)
	}
	if userCount > 0 {
		return "", nil
	}

	// A supplied token is not printed back. The operator already has
	// it, and a script that starts the server would copy it into its log.
	if supplied != "" {
		if len(supplied) < setupTokenMinLength {
			return "", xerrors.Errorf("--setup-token must be at least %d characters", setupTokenMinLength)
		}
		cliui.Infof(inv.Stdout, "A setup token is needed to create the first user. "+
			"It was set with --setup-token or CODER_SETUP_TOKEN.")
		return supplied, nil
	}

	token, err := cryptorand.String(32)
	if err != nil {
		return "", xerrors.Errorf("generate setup token: %w", err)
	}
	cliui.Infof(inv.Stdout, "Setup token: %s\n"+
		"The setup token is needed to create the first user. "+
		"Enter it on the setup page, or when coder login asks for it.", token)
	return token, nil
}
