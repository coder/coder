package poctests_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestAIAgentIdentity is the acceptance test for work package WP1 in
// poc_audit/work_breakdown.md. It is being built incrementally.
//
// Revision 4 completes the path from the workspace to the control plane. A
// startup script launched from the manifest runs an executable, that
// executable calls CreateAIAgent on the workspace_agent over its local socket,
// and the workspace_agent forwards to coderd over its own authenticated
// connection.
//
// It still asserts nothing about AI agent identity, because none of that code
// exists. The stub now lives in coderd, in coderd/agentapi/aiagent.go, where
// it touches a file and returns. Each increment moves that stub one hop
// further from the caller, until the last one replaces it with the behavior
// itself.
//
// What the two hops carry differs, and the difference is the point. The socket
// request names the workspace and presents a credential, because the socket is
// local IPC that authenticates nobody. The coderd request names neither:
// coderd resolved the workspace, its owner, and this workspace_agent while
// authenticating the connection. So the identifiers in the marker were never
// sent by anyone. They are what coderd concluded.
//
// The test passes on two independent conditions: the executable exited zero,
// observed through the script timings the agent reports, and the marker file
// coderd writes holds the identifiers coderd resolved.
//
// The marker has reached its limit here. It survives this hop only because
// this test runs coderd on the same host as the workspace, which no real
// deployment does. The increment that persists an AI agent identity replaces
// it with a database row, which is what the work breakdown calls for anyway.
//
// # Checking that the marker assertion is load-bearing
//
// The two subtests below cover the probe's own failure handling, so they run
// automatically. One check cannot: whether this test would notice if the
// marker stopped being written while the call still succeeded. Go has no
// expect-failure mechanism, so that one is a manual procedure. Run it after
// changing what the handler does or how success is observed.
//
//  1. In coderd/agentapi/aiagent.go, in CreateAIAgent, redirect the write so
//     that no marker appears where the test looks for it:
//
//     os.WriteFile(markerPath+".disabled", ...)
//
//     Redirecting rather than deleting the call keeps the os import used, so
//     the package still builds.
//
//  2. Run: go test ./poc_tests/ -run TestAIAgentIdentity -count=1
//
//  3. Expect ProbeReachesAgentOverSocket to fail on the missing marker, with
//     an empty script log, because the probe exited zero. If it passes, the
//     marker assertion has stopped meaning anything and needs fixing.
//
//  4. Revert the change.
//
// This could be made automatic with the subprocess pattern used in
// agent/reaper/reaper_test.go, re-running the test binary with a fault
// injected and asserting the child fails. That was judged not worth the
// runtime and the fault-injection plumbing while the assertions are this
// simple.
func TestAIAgentIdentity(t *testing.T) {
	t.Parallel()

	t.Run("ProbeReachesAgentOverSocket", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		probeBin := buildProbe(t)
		socketPath := testutil.AgentSocketPath(t)
		markerPath := filepath.Join(t.TempDir(), "probe-marker")

		// The agent writes each script's output under its log directory. Point
		// that at a directory the test owns so a failure can show what the
		// script actually printed, rather than only that it exited non-zero.
		logDir := t.TempDir()
		scriptLogPath := filepath.Join(logDir, "ai-agent-probe.log")

		script, err := os.ReadFile(filepath.Join("testdata", "probe.sh"))
		require.NoError(t, err, "read the startup script")

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			for _, a := range agents {
				a.Scripts = append(a.Scripts, &proto.Script{
					DisplayName: "ai-agent-probe",
					Script:      string(script),
					RunOnStart:  true,
					LogPath:     "ai-agent-probe.log",
				})
			}
			return agents
		}).Do()

		// The agent already exports CODER_WORKSPACE_ID and CODER_AGENT_TOKEN
		// to everything it runs (agent/agent.go:1701 and :1704), so the probe
		// receives those exactly as it would in a real workspace and the test
		// does not supply them.
		//
		// The remaining three have no production source. Nothing exports the
		// socket path today, and the binary and marker paths exist only for
		// this test. Agent-level variables are the mechanism the agent itself
		// uses, and they take precedence over everything else.
		_ = agenttest.New(t, client.URL, r.AgentToken, func(o *agent.Options) {
			// agenttest sets a socket path but leaves the server disabled, so
			// without this there is nothing listening for the probe to dial.
			o.SocketServerEnabled = true
			o.SocketPath = socketPath
			o.LogDir = logDir
			o.EnvironmentVariables = map[string]string{
				"CODER_AGENT_SOCKET_PATH": socketPath,
				"CODER_POC_PROBE_BIN":     probeBin,
				"CODER_POC_MARKER_PATH":   markerPath,
			}
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).AgentNames([]string{}).Wait()

		// The marker is written by the CreateAIAgent handler inside the
		// workspace_agent, so its presence means the call arrived there.
		if !assert.Eventually(t, func() bool {
			_, err := os.Stat(markerPath)
			return err == nil
		}, testutil.WaitLong, testutil.IntervalMedium) {
			t.Fatalf("CreateAIAgent never wrote its marker at %q.\nscript log:\n%s",
				markerPath, readScriptLog(scriptLogPath))
		}

		// coderd writes what it resolved while authenticating the connection,
		// not what the caller sent. Neither identifier below appeared in any
		// request, so matching them against the workspace this test built
		// checks attribution rather than echo.
		require.Len(t, r.Agents, 1, "the fake build should have exactly one agent")
		wantMarker := "CreateAIAgent\n" +
			"workspace_id=" + r.Workspace.ID.String() + "\n" +
			"agent_id=" + r.Agents[0].ID.String() + "\n"

		contents, err := os.ReadFile(markerPath)
		require.NoError(t, err)
		require.Equal(t, wantMarker, string(contents),
			"marker should carry the workspace and agent coderd resolved from the connection")

		// Exit status is checked independently of the marker. The agent
		// reports it, so a probe that wrote the marker and then failed would
		// still be caught.
		//nolint:gocritic // Reading the agent's own reported timings needs a system context.
		systemCtx := dbauthz.AsSystemRestricted(ctx)
		var timing database.GetWorkspaceAgentScriptTimingsByBuildIDRow
		require.Eventually(t, func() bool {
			timings, err := db.GetWorkspaceAgentScriptTimingsByBuildID(systemCtx, r.Build.ID)
			if err != nil || len(timings) == 0 {
				return false
			}
			for _, candidate := range timings {
				if candidate.DisplayName == "ai-agent-probe" {
					timing = candidate
					return true
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalMedium,
			"agent never reported a timing for the probe script")

		require.Equal(t, database.WorkspaceAgentScriptTimingStatusOk, timing.Status,
			"probe script did not complete successfully")
		require.EqualValues(t, 0, timing.ExitCode, "probe script exited non-zero")
	})

	// The probe is what a real workspace will run, so its failure handling is
	// behavior in its own right and not only a check on this test. These run
	// it directly rather than through a workspace: what is under test is the
	// probe's own handling of a broken environment, and standing up an agent
	// would only slow that down.

	t.Run("ProbeFailsWhenSocketIsAbsent", func(t *testing.T) {
		t.Parallel()

		markerPath := filepath.Join(t.TempDir(), "probe-marker")
		absentSocket := filepath.Join(t.TempDir(), "nothing-listening.sock")

		out, err := runProbe(t, map[string]string{
			"CODER_AGENT_SOCKET_PATH": absentSocket,
			"CODER_POC_MARKER_PATH":   markerPath,
			"CODER_WORKSPACE_ID":      uuid.NewString(),
			"CODER_AGENT_TOKEN":       uuid.NewString(),
		})

		require.Error(t, err, "probe should fail when nothing is listening on the socket")
		require.Contains(t, out, "connect to agent socket",
			"failure should name the socket rather than fail obscurely")
		require.NoFileExists(t, markerPath,
			"a failed probe must not leave a marker, or the marker stops meaning success")
	})

	t.Run("ProbeFailsWhenAVariableIsMissing", func(t *testing.T) {
		t.Parallel()

		// Each required variable is dropped in turn. A probe that silently
		// defaulted any of them would pass its own happy path while giving the
		// eventual create-agent call the wrong workspace or credential.
		required := []string{
			"CODER_AGENT_SOCKET_PATH",
			"CODER_POC_MARKER_PATH",
			"CODER_WORKSPACE_ID",
			"CODER_AGENT_TOKEN",
		}

		for _, missing := range required {
			t.Run(missing, func(t *testing.T) {
				t.Parallel()

				markerPath := filepath.Join(t.TempDir(), "probe-marker")
				env := map[string]string{
					"CODER_AGENT_SOCKET_PATH": filepath.Join(t.TempDir(), "unused.sock"),
					"CODER_POC_MARKER_PATH":   markerPath,
					"CODER_WORKSPACE_ID":      uuid.NewString(),
					"CODER_AGENT_TOKEN":       uuid.NewString(),
				}
				delete(env, missing)

				out, err := runProbe(t, env)

				require.Error(t, err, "probe should fail when %s is missing", missing)
				require.Contains(t, out, missing+" is not set",
					"failure should name the missing variable")
				require.NoFileExists(t, markerPath)
			})
		}
	})
}

// readScriptLog returns whatever the startup script wrote, for use in failure
// messages. A test that only reports "the marker is missing" gives no way to
// tell a script that never ran from one that ran and failed.
func readScriptLog(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "(no script log at " + path + ": " + err.Error() + ")"
	}
	if len(contents) == 0 {
		return "(script log is empty)"
	}
	return string(contents)
}

// runProbe runs the probe directly with exactly the given environment and
// returns its combined output. The environment is not inherited, so a variable
// absent from env is genuinely absent.
func runProbe(t *testing.T, env map[string]string) (string, error) {
	t.Helper()

	//nolint:gosec // The path is produced by buildProbe, not by input.
	cmd := exec.Command(buildProbe(t))
	cmd.Env = []string{}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	out, err := cmd.CombinedOutput()
	return string(out), err
}

// buildProbe compiles the probe executable and returns its path. It is built
// rather than run with `go run` so that the startup script invokes a plain
// binary, as it would in a real workspace.
func buildProbe(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "aiagentprobe")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/aiagentprobe")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build the probe executable: %s", out)

	return bin
}
