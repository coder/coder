package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestExperimentRules(t *testing.T) {
	t.Parallel()

	// mcp-tool-search is enabled at startup; example is not.
	setup := func(t *testing.T) (*codersdk.Client, *codersdk.Client, func(args ...string) (string, string, error)) {
		t.Helper()
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: coderdtest.DeploymentValues(t, func(v *codersdk.DeploymentValues) {
				v.Experiments = []string{string(codersdk.ExperimentMCPToolSearch)}
			}),
		})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		run := func(args ...string) (string, string, error) {
			t.Helper()
			inv, root := clitest.New(t, append([]string{"exp", "experiment-rules"}, args...)...)
			clitest.SetupConfig(t, ownerClient, root)
			var stdout, stderr bytes.Buffer
			inv.Stdout = &stdout
			inv.Stderr = &stderr
			err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
			return stdout.String(), stderr.String(), err
		}
		return ownerClient, memberClient, run
	}
	storedRule := func(ctx context.Context, t *testing.T, client *codersdk.Client, ex codersdk.Experiment) *codersdk.ExperimentRule {
		t.Helper()
		entries, err := codersdk.NewExperimentalClient(client).ExperimentRules(ctx)
		require.NoError(t, err)
		for _, entry := range entries {
			if entry.Experiment == string(ex) {
				return entry.Rule
			}
		}
		t.Fatalf("no rules entry for %q", ex)
		return nil
	}
	requireMemberEnabled := func(ctx context.Context, t *testing.T, member *codersdk.Client, ex codersdk.Experiment, want bool) {
		t.Helper()
		got, err := member.Experiments(ctx)
		require.NoError(t, err)
		require.Equal(t, want, got.Enabled(ex))
	}

	// Without --expected-revision, each command writes against the
	// current revision, and the change applies to the next request.
	t.Run("WriteAndList", func(t *testing.T) {
		t.Parallel()
		ownerClient, memberClient, run := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		stdout, _, err := run("on", "example")
		require.NoError(t, err)
		require.Contains(t, stdout, `Experiment "example" rule is now on (revision 1).`)
		requireMemberEnabled(ctx, t, memberClient, codersdk.ExperimentExample, true)

		_, _, err = run("set", "example", `user.username == "owner-only"`)
		require.NoError(t, err)
		requireMemberEnabled(ctx, t, memberClient, codersdk.ExperimentExample, false)

		_, _, err = run("off", "example")
		require.NoError(t, err)
		_, _, err = run("reset", "example")
		require.NoError(t, err)
		rule := storedRule(ctx, t, ownerClient, codersdk.ExperimentExample)
		require.Equal(t, string(codersdk.ExperimentRuleModeInherit), rule.Mode)
		require.Equal(t, int64(4), rule.Revision)

		stdout, _, err = run("list", "-o", "json")
		require.NoError(t, err)
		var entries []codersdk.ExperimentRuleEntry
		require.NoError(t, json.Unmarshal([]byte(stdout), &entries))
		require.Len(t, entries, 2)
		require.Equal(t, rule, entries[0].Rule)

		stdout, _, err = run("list")
		require.NoError(t, err)
		require.Regexp(t, `example\s+false\s+inherit\s+4`, stdout)
		require.Regexp(t, `mcp-tool-search\s+true\s+\(none\)\s+0`, stdout)
	})

	// A stale --expected-revision fails with the current state and is not
	// retried: the stored rule keeps its revision.
	t.Run("ConflictNotRetried", func(t *testing.T) {
		t.Parallel()
		ownerClient, _, run := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, _, err := run("on", "example", "--expected-revision", "0")
		require.NoError(t, err)
		_, stderr, err := run("set", "example", `user.username == "x"`, "--expected-revision", "0")
		require.Error(t, err)
		require.Contains(t, err.Error(), "409")
		require.Contains(t, stderr, `Current rule for experiment "example": on (revision 1)`)
		rule := storedRule(ctx, t, ownerClient, codersdk.ExperimentExample)
		require.Equal(t, string(codersdk.ExperimentRuleModeOn), rule.Mode)
		require.Equal(t, int64(1), rule.Revision)
	})

	// Reset restores the startup default, which may be on, so it warns
	// that it is not a kill switch.
	t.Run("ResetWarnsWhenStaticDefaultOn", func(t *testing.T) {
		t.Parallel()
		_, memberClient, run := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, _, err := run("off", "mcp-tool-search")
		require.NoError(t, err)
		requireMemberEnabled(ctx, t, memberClient, codersdk.ExperimentMCPToolSearch, false)
		_, stderr, err := run("reset", "mcp-tool-search")
		require.NoError(t, err)
		require.Contains(t, stderr, "Reset is not a kill switch")
		requireMemberEnabled(ctx, t, memberClient, codersdk.ExperimentMCPToolSearch, true)

		_, stderr, err = run("reset", "example")
		require.NoError(t, err)
		require.NotContains(t, stderr, "Reset is not a kill switch")
	})
}
