//go:build !slim

package cli

import "github.com/coder/serpent"

// slimEnterpriseStubs returns top-level stub commands for enterprise-only
// features that are not compiled into the slim AGPL build (e.g. the
// homebrew-core formula). The non-slim build has no stubs because the
// real commands are registered by the enterprise binary.
func (*RootCmd) slimEnterpriseStubs() []*serpent.Command {
	return nil
}

// slimEnterpriseProvisionerStubs returns stub subcommands of the AGPL
// `provisioner` command for enterprise-only operations (e.g. `keys`). The
// non-slim build has no stubs; see slimEnterpriseStubs for the rationale.
func (*RootCmd) slimEnterpriseProvisionerStubs() []*serpent.Command {
	return nil
}
