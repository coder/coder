package chatd

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// TestServerStageMetricsFollowExperiment checks that the chat-stage-metrics
// experiment decides whether the stage families reach the server's
// registry. A family with no series is absent from a gather, so one
// observation is recorded first. The server's stage tracer must also
// read the configured clock.
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
			clock := quartz.NewMock(t)
			server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{},
				withInternalTestServerExperiments(experiments),
				withInternalTestServerRegistry(registry),
				withInternalTestServerClock(clock),
			)
			require.Equal(t, clock.Now(), server.stages.Now())
			server.metrics.RecordStageDuration(chatloop.StageCommit, chatloop.ScopeTurn, chatloop.ChatKindRoot, chatloop.StageModel{}, time.Second)

			families, err := registry.Gather()
			require.NoError(t, err)
			var found bool
			for _, family := range families {
				if family.GetName() == "coderd_chatd_stage_duration_seconds" {
					found = true
				}
			}
			require.Equal(t, enabled, found)
		})
	}
}
