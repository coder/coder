package chatd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// TestServerStageMetricsFollowExperiment checks that the chat-stage-metrics
// experiment decides whether the stage families reach the server's
// registry. A family with no series is absent from a gather, so one
// observation is recorded first.
func TestServerStageMetricsFollowExperiment(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			experiments := codersdk.Experiments{}
			if enabled {
				experiments = codersdk.Experiments{codersdk.ExperimentChatStageMetrics}
			}
			server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{},
				withInternalTestServerExperiments(experiments),
				withInternalTestServerRegistry(registry),
			)
			server.metrics.RecordStageDuration(chatloop.StageCommit, chatloop.ScopeTurn, chatloop.ChatKindRoot, chatloop.StageModel{}, time.Second)

			count, err := promtestutil.GatherAndCount(registry, "coderd_chatd_stage_duration_seconds")
			require.NoError(t, err)
			require.Equal(t, enabled, count > 0)
		})
	}
}

// TestServerStageTracerUsesServerConfig checks that the server's stage
// tracer exports to the configured tracer provider and times stages on
// the configured clock.
func TestServerStageTracerUsesServerConfig(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		// t.Context is canceled before Cleanup runs.
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	clock := quartz.NewMock(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{},
		withInternalTestServerClock(clock),
		withInternalTestServerTracerProvider(provider),
	)
	require.Equal(t, clock.Now(), server.stages.Now())

	_, span := server.stages.Start(t.Context(), chatloop.StageCommit)
	clock.Advance(3 * time.Second)
	span.End(nil)

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	require.Equal(t, string(chatloop.StageCommit), ended[0].Name())
	require.Equal(t, 3*time.Second, ended[0].EndTime().Sub(ended[0].StartTime()))
}
