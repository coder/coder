//go:build !slim

package cli

import "github.com/coder/serpent"

// slimEnterpriseStubs returns no commands in non-slim builds. Stubs are
// only registered in slim builds; see enterprise_stubs_slim.go.
func (*RootCmd) slimEnterpriseStubs() []*serpent.Command {
	return nil
}

// slimEnterpriseProvisionerStubs returns no commands in non-slim builds.
// Stubs are only registered in slim builds; see enterprise_stubs_slim.go.
func (*RootCmd) slimEnterpriseProvisionerStubs() []*serpent.Command {
	return nil
}
