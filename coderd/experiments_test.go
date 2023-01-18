package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func Test_Experiments(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentConfig(t)
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentConfig: cfg,
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
		cfg := coderdtest.DeploymentConfig(t)
		cfg.Experiments.Value = []string{"foo", "BAR"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentConfig: cfg,
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
		cfg := coderdtest.DeploymentConfig(t)
		cfg.Experiments.Value = []string{"*"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentConfig: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.ElementsMatch(t, codersdk.ExperimentsAll, experiments)
		for _, ex := range codersdk.ExperimentsAll {
			require.True(t, experiments.Enabled(ex))
		}
		require.False(t, experiments.Enabled("danger"))
	})

	t.Run("alternate wildcard with manual opt-in", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentConfig(t)
		cfg.Experiments.Value = []string{"*", "dAnGeR"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentConfig: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.ElementsMatch(t, append(codersdk.ExperimentsAll, "danger"), experiments)
		for _, ex := range codersdk.ExperimentsAll {
			require.True(t, experiments.Enabled(ex))
		}
		require.True(t, experiments.Enabled("danger"))
		require.False(t, experiments.Enabled("herebedragons"))
	})

	t.Run("legacy wildcard", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentConfig(t)
		cfg.Experimental.Value = true
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentConfig: cfg,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.ElementsMatch(t, codersdk.ExperimentsAll, experiments)
		for _, ex := range codersdk.ExperimentsAll {
			require.True(t, experiments.Enabled(ex))
		}
		require.False(t, experiments.Enabled("danger"))
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentConfig(t)
		cfg.Experiments.Value = []string{"*"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentConfig: cfg,
		})
		// Explicitly omit creating a user so we're unauthorized.
		// _ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Experiments(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, httpmw.SignedOutErrorMessage)
	})
}
