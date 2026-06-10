//go:build slim

package cli

import "github.com/coder/serpent"

// slimEnterpriseStubs returns hidden stub commands for the top-level
// enterprise-only commands that are not compiled into slim AGPL builds.
// Invoking a stub prints guidance on installing a full build of Coder
// instead of failing with "unknown command".
func (*RootCmd) slimEnterpriseStubs() []*serpent.Command {
	return slimEnterpriseStubCommands(EnterpriseCommandStubs())
}

// slimEnterpriseProvisionerStubs returns hidden stub subcommands of the
// AGPL provisioner command for enterprise-only operations such as
// `provisioner keys` and `provisioner start`.
func (*RootCmd) slimEnterpriseProvisionerStubs() []*serpent.Command {
	return slimEnterpriseStubCommands(EnterpriseProvisionerCommandStubs())
}

// slimEnterpriseStubCommands builds hidden commands from stub definitions.
// Each handler prints install guidance and exits non-zero. Leaf stubs set
// RawArgs so any subcommands and flags (e.g. `licenses add -f license.jwt`)
// are absorbed instead of being rejected by the parser. serpent panics when
// a RawArgs command is invoked through an alias, so each alias is registered
// as its own hidden command instead of using the Aliases field.
func slimEnterpriseStubCommands(stubs []EnterpriseCommandStub) []*serpent.Command {
	cmds := make([]*serpent.Command, 0, len(stubs))
	for _, stub := range stubs {
		for _, name := range append([]string{stub.Name}, stub.Aliases...) {
			cmd := &serpent.Command{
				Use:    name,
				Short:  stub.Short,
				Hidden: true,
				Handler: func(inv *serpent.Invocation) error {
					return SlimUnsupported(inv.Stderr, inv.Command.FullName())
				},
			}
			if len(stub.Children) > 0 {
				cmd.Children = slimEnterpriseStubCommands(stub.Children)
			} else {
				cmd.RawArgs = true
			}
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}
