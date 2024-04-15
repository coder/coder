package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
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
		cfg.Experiments = []string{codersdk.ExperimentsAllWildcard}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
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
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{codersdk.ExperimentsAllWildcard, "dAnGeR"}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
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

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{codersdk.ExperimentsAllWildcard}
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
		require.ElementsMatch(t, codersdk.ExperimentsAll, experiments.Safe)
	})

	t.Run("experiments detail", func(t *testing.T) {
		t.Parallel()

		const (
			invalidExp = "bob"
			expiredExp = "auto-fill-parameters" // using a string here not a constant since this experiment has expired & will be deleted eventually
		)

		tests := []struct {
			name               string
			enabledValid       []codersdk.Experiment
			enabledInvalid     []codersdk.Experiment
			expectedExtraCount int
		}{
			{
				name: "using defaults",
			},
			{
				name:         "use all (*)",
				enabledValid: []codersdk.Experiment{codersdk.Experiment(codersdk.ExperimentsAllWildcard)},
			},
			{
				name:         "only valid experiments",
				enabledValid: codersdk.ExperimentsAll,
			},
			{
				name:               "use all (*) + invalid",
				enabledValid:       []codersdk.Experiment{codersdk.Experiment(codersdk.ExperimentsAllWildcard), codersdk.Experiment(expiredExp)},
				expectedExtraCount: 1,
			},
			{
				name:               "valid + expired experiments",
				enabledValid:       codersdk.ExperimentsAll,
				enabledInvalid:     []codersdk.Experiment{codersdk.Experiment(expiredExp)},
				expectedExtraCount: 1,
			},
			{
				name:               "valid + expired + invalid experiments",
				enabledValid:       codersdk.ExperimentsAll,
				enabledInvalid:     []codersdk.Experiment{codersdk.Experiment(invalidExp), codersdk.Experiment(expiredExp)},
				expectedExtraCount: 2,
			},
			{
				name:               "only expired",
				enabledInvalid:     []codersdk.Experiment{codersdk.Experiment(expiredExp)},
				expectedExtraCount: 1,
			},
			{
				name:               "only invalid",
				enabledInvalid:     []codersdk.Experiment{codersdk.Experiment(invalidExp)},
				expectedExtraCount: 1,
			},
			{
				name:               "expired + invalid experiments",
				enabledInvalid:     []codersdk.Experiment{codersdk.Experiment(invalidExp), codersdk.Experiment(expiredExp)},
				expectedExtraCount: 2,
			},
		}

		for _, tc := range tests {
			tc := tc

			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var exps []string

				// given
				for _, e := range tc.enabledValid {
					exps = append(exps, string(e))
				}
				for _, e := range tc.enabledInvalid {
					exps = append(exps, string(e))
				}

				cfg := coderdtest.DeploymentValues(t)
				cfg.Experiments = exps
				client := coderdtest.New(t, &coderdtest.Options{
					DeploymentValues: cfg,
				})
				_ = coderdtest.CreateFirstUser(t, client)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()

				// when
				experiments, err := client.ExperimentDetails(ctx)

				// then
				require.NoError(t, err)
				require.Len(t, experiments, len(codersdk.ExperimentsAll)+tc.expectedExtraCount)
				require.Conditionf(t, func() (success bool) {
					var enabled []bool

					var validCount int
					for _, exp := range tc.enabledValid {
						// don't count wildcard experiment itself as a single experiment
						if exp == codersdk.ExperimentsAllWildcard {
							validCount += len(codersdk.ExperimentsAll)
						} else {
							validCount++
						}
					}

					for _, exp := range append(tc.enabledValid, tc.enabledInvalid...) {
						for _, e := range experiments {
							// * is special-cased to mean all experiments
							if (exp == codersdk.ExperimentsAllWildcard || e.Name == exp) && e.Enabled {
								// codersdk.ExperimentsAllWildcard cannot include invalid experiments
								if exp == codersdk.ExperimentsAllWildcard && e.Invalid {
									continue
								}

								enabled = append(enabled, true)
							}
						}
					}

					return len(enabled) == validCount+len(tc.enabledInvalid)
				}, "enabled experiment(s) were either not found or not marked as enabled")
				require.Conditionf(t, func() (success bool) {
					var invalid []bool
					for _, exp := range tc.enabledInvalid {
						for _, e := range experiments {
							if e.Name == exp && e.Invalid {
								invalid = append(invalid, true)
							}
						}
					}

					return len(invalid) == len(tc.enabledInvalid)
				}, "invalid experiment(s) were either not found or not marked as invalid")
			})
		}
	})
}
