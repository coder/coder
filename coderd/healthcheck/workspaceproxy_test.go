package healthcheck_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceProxies(t *testing.T) {
	t.Parallel()

	t.Run("NotEnabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, nil)
		rpt := healthcheck.WorkspaceProxyReport{}
		rpt.Run(ctx, &healthcheck.WorkspaceProxyReportOptions{
			Logger: log,
		})

		require.True(t, rpt.Healthy, "expected report to be healthy")
		require.Empty(t, rpt.Warnings, "expected no warnings")
		require.Empty(t, rpt.WorkspaceProxies, "expected no proxies")
	})

	t.Run("Enabled/None", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, nil)
		rpt := healthcheck.WorkspaceProxyReport{}
		rpt.Run(ctx, &healthcheck.WorkspaceProxyReportOptions{
			CurrentVersion: "v2.34.5",
			FetchWorkspaceProxies: func(_ context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
				return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
					Regions: []codersdk.WorkspaceProxy{},
				}, nil
			},
			UpdateProxyHealth: func(context.Context) error { return nil },
			Logger:            log,
		})

		require.True(t, rpt.Healthy, "expected report to be healthy")
		require.Empty(t, rpt.Warnings, "expected no warnings")
		require.NotEmpty(t, rpt.WorkspaceProxies, "expected at least one proxy")
	})

	t.Run("Enabled/Match", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, nil)
		rpt := healthcheck.WorkspaceProxyReport{}
		rpt.Run(ctx, &healthcheck.WorkspaceProxyReportOptions{
			CurrentVersion: "v2.34.5",
			FetchWorkspaceProxies: func(_ context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
				return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
					Regions: []codersdk.WorkspaceProxy{
						fakeWorkspaceProxy(true, "v2.34.5"),
						fakeWorkspaceProxy(true, "v2.34.5"),
					},
				}, nil
			},
			UpdateProxyHealth: func(context.Context) error { return nil },
			Logger:            log,
		})

		require.True(t, rpt.Healthy, "expected report to be healthy")
		require.Empty(t, rpt.Warnings, "expected no warnings")
		require.NotEmpty(t, rpt.WorkspaceProxies, "expected at least one proxy")
	})

	t.Run("Enabled/Mismatch/One", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, nil)
		rpt := healthcheck.WorkspaceProxyReport{}
		rpt.Run(ctx, &healthcheck.WorkspaceProxyReportOptions{
			CurrentVersion: "v2.35.0",
			FetchWorkspaceProxies: func(_ context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
				return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
					Regions: []codersdk.WorkspaceProxy{
						fakeWorkspaceProxy(true, "v2.35.0"),
						fakeWorkspaceProxy(true, "v2.34.5"),
					},
				}, nil
			},
			Logger:            log,
			UpdateProxyHealth: func(context.Context) error { return nil },
		})

		require.False(t, rpt.Healthy, "expected report not to be healthy")
		require.Len(t, rpt.Warnings, 1)
		require.Contains(t, rpt.Warnings[0], "does not match primary server version")
		require.NotEmpty(t, rpt.WorkspaceProxies)
	})

	t.Run("Enabled/Mismatch/Multiple", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, nil)
		rpt := healthcheck.WorkspaceProxyReport{}
		rpt.Run(ctx, &healthcheck.WorkspaceProxyReportOptions{
			CurrentVersion: "v2.35.0",
			FetchWorkspaceProxies: func(_ context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
				return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
					Regions: []codersdk.WorkspaceProxy{
						fakeWorkspaceProxy(true, "v2.34.5"),
						fakeWorkspaceProxy(true, "v2.34.5"),
					},
				}, nil
			},
			Logger:            log,
			UpdateProxyHealth: func(context.Context) error { return nil },
		})

		require.False(t, rpt.Healthy, "expected report not to be healthy")
		require.Len(t, rpt.Warnings, 2)
		require.Contains(t, rpt.Warnings[0], "does not match primary server version")
		require.Contains(t, rpt.Warnings[1], "does not match primary server version")
		require.NotEmpty(t, rpt.WorkspaceProxies)
	})
}

func fakeWorkspaceProxy(healthy bool, version string) codersdk.WorkspaceProxy {
	return codersdk.WorkspaceProxy{
		Region: codersdk.Region{
			Healthy: healthy,
		},
		Version: version,
	}
}
