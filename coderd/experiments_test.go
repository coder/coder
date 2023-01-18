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

// nolint:paralleltest  // t.Setenv
func Test_Experiments(t *testing.T) {
	// In these tests we set the environment variable directly, rather than
	// modifying the deployment config. This corresponds more directly to what
	// an end user would do. Also, modifying the deployment config doesn't work
	// for expanding the wildcard because of how viper binds stuff.
	t.Run("empty", func(t *testing.T) {
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.Empty(t, experiments)
		require.False(t, experiments.Enabled(codersdk.ExperimentVSCodeLocal))
		require.False(t, experiments.Enabled("foo"))
	})

	t.Run("multiple features", func(t *testing.T) {
		t.Setenv("CODER_EXPERIMENTAL", "foo,bar")
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		experiments, err := client.Experiments(ctx)
		require.NoError(t, err)
		require.NotNil(t, experiments)
		require.ElementsMatch(t, []codersdk.Experiment{"foo", "bar"}, experiments)
		require.True(t, experiments.Enabled("foo"))
		require.True(t, experiments.Enabled("bar"))
		require.False(t, experiments.Enabled("baz"))
	})

	t.Run("wildcard", func(t *testing.T) {
		t.Setenv("CODER_EXPERIMENTAL", "*")
		client := coderdtest.New(t, nil)
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
		t.Setenv("CODER_EXPERIMENTAL", "true,danger")
		client := coderdtest.New(t, nil)
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

	t.Run("Unauthorized", func(t *testing.T) {
		t.Setenv("CODER_EXPERIMENTAL", "foo,bar")
		client := coderdtest.New(t, nil)
		// Explicitly omit creating a user so we're unauthorized.
		// _ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Experiments(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, httpmw.SignedOutErrorMessage)
	})
}
