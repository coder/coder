package cli

// EnterpriseCommandStub describes an enterprise-only command that slim
// AGPL builds register as a hidden stub. Invoking a stub prints guidance
// on installing a full build of Coder instead of failing with "unknown
// command".
//
// The table lives in an untagged file so it compiles in every build and
// tests can assert it stays in sync with the enterprise CLI without
// requiring -tags slim.
type EnterpriseCommandStub struct {
	Name    string
	Short   string
	Aliases []string
	// Children holds enterprise-only subcommands stubbed beneath this
	// command.
	Children []EnterpriseCommandStub
}

// EnterpriseCommandStubs returns the top-level enterprise-only commands
// stubbed in slim AGPL builds. Names, short descriptions, and aliases
// must mirror the real commands registered by enterprise/cli; a test in
// enterprise/cli asserts parity with the enterprise command list.
func EnterpriseCommandStubs() []EnterpriseCommandStub {
	return []EnterpriseCommandStub{
		{Name: "agent-firewall", Short: "Network isolation tool for monitoring and restricting HTTP/HTTPS requests"},
		{Name: "workspace-proxy", Short: "Workspace proxies provide low-latency experiences for geo-distributed teams.", Aliases: []string{"wsproxy"}},
		{Name: "features", Short: "List Enterprise features", Aliases: []string{"feature"}},
		{Name: "licenses", Short: "Add, delete, and list licenses", Aliases: []string{"license"}},
		{Name: "groups", Short: "Manage groups", Aliases: []string{"group"}},
		{Name: "prebuilds", Short: "Manage Coder prebuilds", Aliases: []string{"prebuild"}},
		{Name: "external-workspaces", Short: "Create or manage external workspaces"},
		{Name: "aibridge", Short: "Manage AI Bridge."},
	}
}

// EnterpriseProvisionerCommandStubs returns the enterprise-only
// subcommands of the provisioner command stubbed in slim AGPL builds. The
// AGPL provisioner command keeps its real list and jobs subcommands.
func EnterpriseProvisionerCommandStubs() []EnterpriseCommandStub {
	return []EnterpriseCommandStub{
		{Name: "keys", Short: "Manage provisioner keys", Aliases: []string{"key"}},
		{Name: "start", Short: "Run a provisioner daemon"},
	}
}
