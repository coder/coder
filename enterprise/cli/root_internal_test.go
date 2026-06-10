package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/serpent"
)

//nolint:tparallel,paralleltest
func TestEnterpriseCommandHelp(t *testing.T) {
	// Only test the enterprise commands
	getCmds := func(t *testing.T) *serpent.Command {
		// Must return a fresh instance of cmds each time.
		t.Helper()
		var root cli.RootCmd
		rootCmd, err := root.Command((&RootCmd{}).enterpriseOnly())
		require.NoError(t, err)

		return rootCmd
	}
	clitest.TestCommandHelp(t, getCmds, clitest.DefaultCases())
}

// TestSlimEnterpriseCommandStubs asserts the stub table registered by slim
// AGPL builds stays in sync with the enterprise command list: every
// enterprise-only command has a stub with matching name, short text, and
// aliases, and no stale stubs remain.
func TestSlimEnterpriseCommandStubs(t *testing.T) {
	t.Parallel()

	stubIndex := func(t *testing.T, stubs []cli.EnterpriseCommandStub) map[string]cli.EnterpriseCommandStub {
		t.Helper()
		index := make(map[string]cli.EnterpriseCommandStub, len(stubs))
		for _, stub := range stubs {
			_, ok := index[stub.Name]
			require.False(t, ok, "duplicate stub %q", stub.Name)
			index[stub.Name] = stub
		}
		return index
	}
	requireStubMatches := func(t *testing.T, cmd *serpent.Command, stub cli.EnterpriseCommandStub) {
		t.Helper()
		require.Equal(t, cmd.Short, stub.Short, "stub %q short text must mirror the enterprise command", stub.Name)
		require.ElementsMatch(t, cmd.Aliases, stub.Aliases, "stub %q aliases must mirror the enterprise command", stub.Name)
	}

	t.Run("TopLevel", func(t *testing.T) {
		t.Parallel()

		// Commands without a top-level stub: server, provisioner, and exp
		// exist in AGPL builds with their own slim handling, while boundary
		// and provisionerd are hidden deprecated wrappers.
		skip := map[string]bool{
			"server":       true,
			"provisioner":  true,
			"exp":          true,
			"boundary":     true,
			"provisionerd": true,
		}
		stubs := stubIndex(t, cli.EnterpriseCommandStubs())
		seen := make(map[string]bool)
		for _, cmd := range (&RootCmd{}).enterpriseOnly() {
			name := cmd.Name()
			if skip[name] {
				continue
			}
			stub, ok := stubs[name]
			require.True(t, ok, "enterprise command %q has no slim stub; add it to cli.EnterpriseCommandStubs", name)
			requireStubMatches(t, cmd, stub)
			seen[name] = true
		}
		for name := range stubs {
			require.True(t, seen[name], "stale slim stub %q matches no enterprise command", name)
		}
	})

	t.Run("Provisioner", func(t *testing.T) {
		t.Parallel()

		// The provisioner command exists in AGPL builds with its own
		// subcommands; only the enterprise-only subcommands are stubbed.
		agplChildren := make(map[string]bool)
		for _, cmd := range (&cli.RootCmd{}).Provisioners().Children {
			agplChildren[cmd.Name()] = true
		}
		stubs := stubIndex(t, cli.EnterpriseProvisionerCommandStubs())
		seen := make(map[string]bool)
		for _, cmd := range (&RootCmd{}).provisionerDaemons().Children {
			name := cmd.Name()
			if agplChildren[name] {
				continue
			}
			stub, ok := stubs[name]
			require.True(t, ok, "enterprise provisioner subcommand %q has no slim stub; add it to cli.EnterpriseProvisionerCommandStubs", name)
			requireStubMatches(t, cmd, stub)
			seen[name] = true
		}
		for name := range stubs {
			require.True(t, seen[name], "stale slim provisioner stub %q matches no enterprise provisioner subcommand", name)
		}
	})
}
