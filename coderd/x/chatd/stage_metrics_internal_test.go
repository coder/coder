package chatd

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

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
			// A family with no series is absent from a gather.
			_, span := chatloop.NewStageTracer(nil, server.metrics).Start(t.Context(), chatloop.StageCommit)
			span.End(nil)

			count, err := promtestutil.GatherAndCount(registry, "coderd_chatd_stage_duration_seconds")
			require.NoError(t, err)
			require.Equal(t, enabled, count > 0)
		})
	}
}
