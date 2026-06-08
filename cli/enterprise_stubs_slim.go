//go:build slim

package cli

import "github.com/coder/serpent"

// slimEnterpriseStubs returns top-level stub commands for enterprise-only
// features that are not compiled into the slim AGPL build (e.g. the
// homebrew-core formula). Each stub prints a helpful error pointing the
// user at the full Coder build instead of failing with "unknown command".
//
// The list mirrors `(*enterprise/cli.RootCmd).enterpriseOnly()` minus
// commands that already exist or are stubbed elsewhere in AGPL (`server`,
// `provisioner start`) and minus hidden/deprecated wrappers (`provisionerd`,
// `boundary`).
func (*RootCmd) slimEnterpriseStubs() []*serpent.Command {
	return []*serpent.Command{
		slimEnterpriseStub("licenses", "Add, delete, and list licenses", "license"),
		slimEnterpriseStub("groups", "Manage groups", "group"),
		slimEnterpriseStub("prebuilds", "Manage Coder prebuilds"),
		slimEnterpriseStub("features", "List Coder Enterprise features", "feature"),
		slimEnterpriseStub("workspace-proxy", "Manage workspace proxies"),
		slimEnterpriseStub("external-workspaces", "Create or manage external workspaces"),
		slimEnterpriseStub("aibridge", "Manage AI Bridge"),
		slimEnterpriseStub("agent-firewall", "Manage agent firewalls"),
	}
}

// slimEnterpriseProvisionerStubs returns stub subcommands of the AGPL
// `provisioner` command. This is what fires for `coder provisioner keys ...`
// on the homebrew-core binary, which was Kayla's original pain point.
func (*RootCmd) slimEnterpriseProvisionerStubs() []*serpent.Command {
	return []*serpent.Command{
		slimEnterpriseStub("keys", "Manage provisioner keys", "key"),
	}
}

// slimEnterpriseStub returns a command that absorbs any arguments and
// prints slimEnterpriseUnsupportedMsg on invocation. RawArgs lets the stub
// accept further subcommands (e.g. `licenses add`) without the serpent
// parser rejecting them as unknown.
func slimEnterpriseStub(use, short string, aliases ...string) *serpent.Command {
	return &serpent.Command{
		Use:     use,
		Short:   short,
		Aliases: aliases,
		RawArgs: true,
		Handler: func(inv *serpent.Invocation) error {
			slimEnterpriseUnsupportedMsg(inv.Stderr, inv.Command.FullName())
			return ErrSilent
		},
	}
}
