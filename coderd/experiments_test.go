package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	experimentrules "github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_Experiments(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.Empty(t, experiments)
		require.False(t, experiments.Enabled("foo"))
	})

	t.Run("multiple features", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"foo", "BAR"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		// Should be lower-cased.
		require.ElementsMatch(t, []codersdk.Experiment{"foo", "bar"}, experiments)
		require.True(t, experiments.Enabled("foo"))
		require.True(t, experiments.Enabled("bar"))
		require.False(t, experiments.Enabled("baz"))
	})

	t.Run("wildcard", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"*"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.ElementsMatch(t, codersdk.ExperimentsSafe, experiments)
		for _, ex := range codersdk.ExperimentsSafe {
			require.True(t, experiments.Enabled(ex))
		}
		require.False(t, experiments.Enabled("danger"))
	})

	t.Run("alternate wildcard with manual opt-in", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"*", "dAnGeR"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.ElementsMatch(t, append(codersdk.ExperimentsSafe, "danger"), experiments)
		for _, ex := range codersdk.ExperimentsSafe {
			require.True(t, experiments.Enabled(ex))
		}
		require.True(t, experiments.Enabled("danger"))
		require.False(t, experiments.Enabled("herebedragons"))
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"*"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
		})
		// Explicitly omit creating a user so we're unauthorized.
		// _ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Experiments(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, httpmw.SignedOutErrorMessage)
	})

	t.Run("rules per user", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"foo", string(codersdk.ExperimentMCPToolSearch)}
		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			DeploymentValues: cfg,
		})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		revisions := map[codersdk.Experiment]int64{}
		writeRule := func(ex codersdk.Experiment, rule experimentrules.Rule) {
			t.Helper()
			_, stored, _, err := experimentrules.WriteRule(dbauthz.AsSystemRestricted(ctx), db, owner.UserID, ex, rule, revisions[ex])
			require.NoError(t, err)
			revisions[ex] = stored.Revision
		}
		requireExperiments := func(client *codersdk.Client, want ...codersdk.Experiment) {
			t.Helper()
			got, err := client.Experiments(ctx)
			require.NoError(t, err)
			require.ElementsMatch(t, want, got)
		}

		// A condition enables a user-scoped experiment only for matching
		// users; a kill switch disables a statically enabled one.
		writeRule(codersdk.ExperimentExample, experimentrules.Rule{
			Mode:      experimentrules.ModeCondition,
			Condition: fmt.Sprintf("user.username == %q", member.Username),
		})
		writeRule(codersdk.ExperimentMCPToolSearch, experimentrules.Rule{Mode: experimentrules.ModeOff})
		requireExperiments(memberClient, "foo", codersdk.ExperimentExample)
		requireExperiments(ownerClient, "foo")

		// Inherit restores the startup default.
		writeRule(codersdk.ExperimentMCPToolSearch, experimentrules.Rule{Mode: experimentrules.ModeInherit})
		requireExperiments(memberClient, "foo", codersdk.ExperimentMCPToolSearch, codersdk.ExperimentExample)
		requireExperiments(ownerClient, "foo", codersdk.ExperimentMCPToolSearch)

		// The personalized result must not be cached.
		res, err := memberClient.Request(ctx, http.MethodGet, "/api/v2/experiments", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	})

	t.Run("available experiments", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.SafeExperiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.ElementsMatch(t, codersdk.ExperimentsSafe, experiments.Safe)
	})
}
