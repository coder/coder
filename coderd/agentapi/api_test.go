package agentapi_test

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_APIClose(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	log := slogtest.Make(t, nil)

	db, pubsub := dbtestutil.NewDB(t)
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)

	mockTemplateScheduleStore := schedule.MockTemplateScheduleStore{}
	var templateScheduleStore schedule.TemplateScheduleStore = mockTemplateScheduleStore
	templateScheduleStorePtr := atomic.Pointer[schedule.TemplateScheduleStore]{}
	templateScheduleStorePtr.Store(&templateScheduleStore)

	statsBatcher, closeBatcher, err := workspacestats.NewBatcher(ctx, workspacestats.BatcherWithStore(db))
	require.NoError(t, err)
	t.Cleanup(closeBatcher)
	statsTracker := workspacestats.NewTracker(db)
	t.Cleanup(func() {
		_ = statsTracker.Close()
	})
	statsReporter := workspacestats.NewReporter(workspacestats.ReporterOptions{
		Database:              db,
		Logger:                log,
		Pubsub:                pubsub,
		TemplateScheduleStore: &templateScheduleStorePtr,
		StatsBatcher:          statsBatcher,
		UsageTracker:          statsTracker,
		UpdateAgentMetricsFn:  func(_ context.Context, _ prometheusmetrics.AgentMetricLabels, _ []*proto.Stats_Metric) {},
		AppStatBatchSize:      0,
	})

	appearanceFetcherPtr := atomic.Pointer[appearance.Fetcher]{}
	appearanceFetcherPtr.Store(&appearance.DefaultFetcher)

	api := agentapi.New(agentapi.Options{
		AgentID:  uuid.New(),
		Ctx:      ctx,
		Log:      log,
		Database: db,
		Pubsub:   pubsub,
		DerpMapFn: func() *tailcfg.DERPMap {
			return &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{999: {RegionCode: "test"}}}
		},
		TailnetCoordinator:       &coordPtr,
		StatsReporter:            statsReporter,
		AppearanceFetcher:        &appearanceFetcherPtr,
		PublishWorkspaceUpdateFn: func(_ context.Context, _ uuid.UUID) {},
		NetworkTelemetryBatchFn:  func(_ []telemetry.NetworkEvent) {},
		AccessURL: &url.URL{
			Scheme: "http",
			Host:   "localhost",
		},
		AppHostname:                    "",
		AgentStatsRefreshInterval:      time.Second,
		DisableDirectConnections:       false,
		DerpForceWebSockets:            false,
		DerpMapUpdateFrequency:         time.Second,
		NetworkTelemetryBatchFrequency: time.Second,
		NetworkTelemetryBatchMaxSize:   1,
		ExternalAuthConfigs:            []*externalauth.Config{},
		Experiments:                    codersdk.Experiments{},
		WorkspaceID:                    uuid.New(),
		UpdateAgentMetricsFn:           func(_ context.Context, _ prometheusmetrics.AgentMetricLabels, _ []*proto.Stats_Metric) {},
	})

	err = api.Close()
	require.NoError(t, err)
}
