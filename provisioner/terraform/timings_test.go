//go:build linux || darwin

package terraform_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
	terraform_internal "github.com/coder/coder/v2/provisioner/terraform/internal"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestTimingsFromProvision uses a fake terraform binary which spits out expected log content.
// This log content is then used to usher the provisioning process along as if terraform has run, and consequently
// the timing data is extracted from the log content and validated against the expected values.
func TestTimingsFromProvision(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Given: a fake terraform bin that behaves as we expect it to.
	fakeBin := filepath.Join(cwd, "testdata", "timings-aggregation/fake-terraform.sh")

	t.Log(fakeBin)

	ctx, api := setupProvisioner(t, &provisionerServeOptions{
		binaryPath: fakeBin,
	})
	sess := configure(ctx, t, api, &proto.Config{})

	ctx, cancel := context.WithTimeout(ctx, testutil.WaitLong)
	t.Cleanup(cancel)

	var timings []*proto.Timing

	handleResponse := func(t *testing.T, stage string) {
		t.Helper()
		for {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			default:
			}

			msg, err := sess.Recv()
			require.NoError(t, err)

			if log := msg.GetLog(); log != nil {
				t.Logf("%s: %s: %s", stage, log.Level.String(), log.Output)
				continue
			}
			switch {
			case msg.GetInit() != nil:
				timings = append(timings, msg.GetInit().GetTimings()...)
			case msg.GetPlan() != nil:
				timings = append(timings, msg.GetPlan().GetTimings()...)
			case msg.GetApply() != nil:
				timings = append(timings, msg.GetApply().GetTimings()...)
			case msg.GetGraph() != nil:
				timings = append(timings, msg.GetGraph().GetTimings()...)
			}
			break
		}
	}

	// When: configured, our fake terraform will fake an init setup
	err = sendInit(sess, testutil.CreateTar(t, nil))
	require.NoError(t, err)
	handleResponse(t, "init")

	// When: a plan is executed in the provisioner, our fake terraform will be executed and will produce a
	// state file and some log content.
	err = sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	handleResponse(t, "plan")

	// When: the plan has completed, let's trigger an apply.
	err = sendApply(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	handleResponse(t, "apply")

	// When: the apply has completed, graph the results
	err = sendGraph(sess, proto.GraphSource_SOURCE_STATE)
	require.NoError(t, err)

	handleResponse(t, "graph")

	// Sort the timings stably to keep reduce flakiness.
	terraform_internal.StableSortTimings(t, timings)
	// `coder_stage_` timings use `dbtime.Now()`, which makes them hard to compare to
	// a static set of expected timings. Filter them out. This test is good for
	// testing timings sourced from terraform logs, not internal coder timings.
	timings = slice.Filter(timings, func(tim *proto.Timing) bool {
		return !strings.HasPrefix(tim.Resource, "coder_stage_")
	})

	// Then: the received timings should match the expected values below.
	// NOTE: These timings have been encoded to JSON format to make the tests more readable.
	initTimings := terraform_internal.ParseTimingLines(t, []byte(`{"start":"2025-10-22T17:48:29Z","end":"2025-10-22T17:48:31Z","action":"load","resource":"modules","stage":"init","state":"COMPLETED"}
{"start":"2025-10-22T17:48:29Z","end":"2025-10-22T17:48:29Z","action":"load","resource":"backend","stage":"init","state":"COMPLETED"}
{"start":"2025-10-22T17:48:31Z","end":"2025-10-22T17:48:34Z","action":"load","resource":"provider plugins","stage":"init","state":"COMPLETED"}`))
	planTimings := terraform_internal.ParseTimingLines(t, []byte(`{"start":"2024-08-15T08:26:39.194726Z", "end":"2024-08-15T08:26:39.195836Z", "action":"read", "source":"coder", "resource":"data.coder_parameter.memory_size", "stage":"plan", "state":"COMPLETED"}
{"start":"2024-08-15T08:26:39.194726Z", "end":"2024-08-15T08:26:39.195712Z", "action":"read", "source":"coder", "resource":"data.coder_provisioner.me", "stage":"plan", "state":"COMPLETED"}
{"start":"2024-08-15T08:26:39.194726Z", "end":"2024-08-15T08:26:39.195820Z", "action":"read", "source":"coder", "resource":"data.coder_workspace.me", "stage":"plan", "state":"COMPLETED"}`))
	applyTimings := terraform_internal.ParseTimingLines(t, []byte(`{"start":"2024-08-15T08:26:39.616546Z", "end":"2024-08-15T08:26:39.618045Z", "action":"create", "source":"coder", "resource":"coder_agent.main", "stage":"apply", "state":"COMPLETED"}
{"start":"2024-08-15T08:26:39.626722Z", "end":"2024-08-15T08:26:39.669954Z", "action":"create", "source":"docker", "resource":"docker_image.main", "stage":"apply", "state":"COMPLETED"}
{"start":"2024-08-15T08:26:39.627335Z", "end":"2024-08-15T08:26:39.660616Z", "action":"create", "source":"docker", "resource":"docker_volume.home_volume", "stage":"apply", "state":"COMPLETED"}
{"start":"2024-08-15T08:26:39.682223Z", "end":"2024-08-15T08:26:40.186482Z", "action":"create", "source":"docker", "resource":"docker_container.workspace[0]", "stage":"apply", "state":"COMPLETED"}`))
	// Graphing is omitted as it is captured by the stage timing, which uses now()

	totals := make(map[string]int)
	for _, ti := range timings {
		totals[ti.Stage]++
	}
	require.Equal(t, len(initTimings), totals["init"], "init")
	require.Equal(t, len(planTimings), totals["plan"], "plan")
	require.Equal(t, len(applyTimings), totals["apply"], "apply")

	// Lastly total
	require.Len(t, timings, len(initTimings)+len(planTimings)+len(applyTimings))

	// init/graph timings are computed dynamically during provisioning whereas plan/apply come from the logs (fixtures) in
	// provisioner/terraform/testdata/timings-aggregation/fake-terraform.sh.
	//
	// This walks the timings, keeping separate cursors for plan and apply.
	// We manually override the init/graph timings' timestamps so that the equality check works (all other fields should be as expected).
	pCursor := 0
	aCursor := 0
	iCursor := 0
	for _, tim := range timings {
		switch tim.Stage {
		case string(database.ProvisionerJobTimingStageInit):
			require.True(t, terraform_internal.TimingsAreEqual(t, []*proto.Timing{initTimings[iCursor]}, []*proto.Timing{tim}))
			iCursor++
		case string(database.ProvisionerJobTimingStagePlan):
			require.True(t, terraform_internal.TimingsAreEqual(t, []*proto.Timing{planTimings[pCursor]}, []*proto.Timing{tim}))
			pCursor++
		case string(database.ProvisionerJobTimingStageApply):
			require.True(t, terraform_internal.TimingsAreEqual(t, []*proto.Timing{applyTimings[aCursor]}, []*proto.Timing{tim}))
			aCursor++
		}
	}
}
