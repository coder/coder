package cli

import (
	"net/http"
	"os"
	"runtime/debug"

	"golang.org/x/xerrors"

	boundarycli "github.com/coder/boundary/cli"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func isChild() bool {
	return os.Getenv("CHILD") == "true"
}

func getBoundaryVersion() string {
	const boundaryModulePath = "github.com/coder/boundary"

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	for _, module := range buildInfo.Deps {
		if module.Path == boundaryModulePath {
			return module.Version
		}
	}

	return "unknown"
}

func (r *RootCmd) verifyLicense(inv *serpent.Invocation) error {
	client, err := r.InitClient(inv)
	if err != nil {
		return err
	}

	entitlements, err := client.Entitlements(inv.Context())
	if cerr, ok := codersdk.AsError(err); ok && cerr.StatusCode() == http.StatusNotFound {
		return xerrors.Errorf("your deployment appears to be an AGPL deployment, so you cannot use the boundary command")
	} else if err != nil {
		return xerrors.Errorf("failed to get entitlements: %w", err)
	}

	feature := entitlements.Features[codersdk.FeatureBoundary]
	if feature.Entitlement == codersdk.EntitlementNotEntitled {
		return xerrors.Errorf("your license is not entitled to use the boundary feature")
	}
	if !feature.Enabled {
		// Feature is entitled but disabled (shouldn't happen for FeatureBoundary
		// since it's in AlwaysEnable(), but handle it gracefully).
		return xerrors.Errorf("the boundary feature is disabled in your deployment configuration")
	}

	return nil
}

func (r *RootCmd) boundary() *serpent.Command {
	version := getBoundaryVersion()
	cmd := boundarycli.BaseCommand(version) // Package coder/boundary/cli exports a "base command" designed to be integrated as a subcommand.
	cmd.Use += " [args...]"                 // The base command looks like `boundary -- command`. Serpent adds the flags piece, but we need to add the args.

	// Wrap the handler to check for FeatureBoundary entitlement.
	originalHandler := cmd.Handler
	cmd.Handler = func(inv *serpent.Invocation) error {
		// Boundary re-executes itself with CHILD=true to run the target process
		// inside a jailed network namespace. Skip the license check for child
		// processes since the parent already verified entitlement.
		if isChild() {
			return originalHandler(inv)
		}

		if err := r.verifyLicense(inv); err != nil {
			return err
		}

		// Call the original handler if entitlement check passes.
		return originalHandler(inv)
	}

	return cmd
}
