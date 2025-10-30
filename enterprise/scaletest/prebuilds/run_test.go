package prebuilds_test

import (
	"io"
	"strconv"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/scaletest/prebuilds"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Skip("This test takes several minutes to run, and is intended as a manual regression test")

	ctx := testutil.Context(t, testutil.WaitSuperLong*3)

	client, user := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspacePrebuilds:         1,
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})

	// This is a real Terraform provisioner
	_ = coderdenttest.NewExternalProvisionerDaemonTerraform(t, client, user.OrganizationID, nil)

	numTemplates := 2
	numPresets := 1
	numPresetPrebuilds := 1

	//nolint:gocritic // It's fine to use the owner user to pause prebuilds
	err := client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
		ReconciliationPaused: true,
	})
	require.NoError(t, err)

	setupBarrier := new(sync.WaitGroup)
	setupBarrier.Add(numTemplates)
	deletionBarrier := new(sync.WaitGroup)
	deletionBarrier.Add(numTemplates)

	metrics := prebuilds.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	runners := make([]*prebuilds.Runner, 0, numTemplates)
	for i := range numTemplates {
		cfg := prebuilds.Config{
			OrganizationID:            user.OrganizationID,
			NumPresets:                numPresets,
			NumPresetPrebuilds:        numPresetPrebuilds,
			TemplateVersionJobTimeout: testutil.WaitSuperLong * 2,
			PrebuildWorkspaceTimeout:  testutil.WaitSuperLong * 2,
			Metrics:                   metrics,
			SetupBarrier:              setupBarrier,
			DeletionBarrier:           deletionBarrier,
			Clock:                     quartz.NewReal(),
		}
		err := cfg.Validate()
		require.NoError(t, err)

		runner := prebuilds.NewRunner(client, cfg)
		runners = append(runners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, strconv.Itoa(i), io.Discard)
		})
	}

	// Wait for all runners to reach the setup barrier (templates created)
	setupBarrier.Wait()

	// Resume prebuilds to trigger prebuild creation
	err = client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
		ReconciliationPaused: false,
	})
	require.NoError(t, err)

	err = eg.Wait()
	require.NoError(t, err)

	//nolint:gocritic // Owner user is fine here as we want to view all workspaces
	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	expectedWorkspaces := numTemplates * numPresets * numPresetPrebuilds
	require.Equal(t, workspaces.Count, expectedWorkspaces)

	// Now run Cleanup which measures deletion
	// First pause prebuilds again
	err = client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
		ReconciliationPaused: true,
	})
	require.NoError(t, err)

	cleanupEg, cleanupCtx := errgroup.WithContext(ctx)
	for i, runner := range runners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, strconv.Itoa(i), io.Discard)
		})
	}

	// Wait for all runners to reach the deletion barrier (template versions updated to 0 prebuilds)
	deletionBarrier.Wait()

	// Resume prebuilds to trigger prebuild deletion
	err = client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
		ReconciliationPaused: false,
	})
	require.NoError(t, err)

	err = cleanupEg.Wait()
	require.NoError(t, err)

	// Verify all prebuild workspaces were deleted
	workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Equal(t, workspaces.Count, 0)

	for _, runner := range runners {
		metrics := runner.GetMetrics()

		require.Contains(t, metrics, prebuilds.PrebuildsTotalLatencyMetric)
		require.Contains(t, metrics, prebuilds.PrebuildJobCreationLatencyMetric)
		require.Contains(t, metrics, prebuilds.PrebuildJobAcquiredLatencyMetric)

		creationLatency, ok := metrics[prebuilds.PrebuildsTotalLatencyMetric].(int64)
		require.True(t, ok)
		jobCreationLatency, ok := metrics[prebuilds.PrebuildJobCreationLatencyMetric].(int64)
		require.True(t, ok)
		jobAcquiredLatency, ok := metrics[prebuilds.PrebuildJobAcquiredLatencyMetric].(int64)
		require.True(t, ok)

		require.Greater(t, creationLatency, int64(0))
		require.Greater(t, jobCreationLatency, int64(0))
		require.Greater(t, jobAcquiredLatency, int64(0))

		require.Contains(t, metrics, prebuilds.PrebuildDeletionTotalLatencyMetric)
		require.Contains(t, metrics, prebuilds.PrebuildDeletionJobCreationLatencyMetric)
		require.Contains(t, metrics, prebuilds.PrebuildDeletionJobAcquiredLatencyMetric)

		deletionLatency, ok := metrics[prebuilds.PrebuildDeletionTotalLatencyMetric].(int64)
		require.True(t, ok)
		deletionJobCreationLatency, ok := metrics[prebuilds.PrebuildDeletionJobCreationLatencyMetric].(int64)
		require.True(t, ok)
		deletionJobAcquiredLatency, ok := metrics[prebuilds.PrebuildDeletionJobAcquiredLatencyMetric].(int64)
		require.True(t, ok)

		require.Greater(t, deletionLatency, int64(0))
		require.Greater(t, deletionJobCreationLatency, int64(0))
		require.Greater(t, deletionJobAcquiredLatency, int64(0))
	}
}
