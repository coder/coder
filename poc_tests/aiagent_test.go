package poctests_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestAIAgentIdentity is the acceptance test for work package WP1 in
// poc_audit/work_breakdown.md. It is being built incrementally.
//
// Revision 5 removes the last of the scaffolding. A startup script launched
// from the manifest runs an executable, that executable calls CreateAIAgent on
// the workspace_agent over its local socket, the workspace_agent forwards to
// coderd over its own authenticated connection, and coderd mints an identity
// for the AI agent and journals its creation.
//
// Every earlier revision observed the call by having whichever handler it had
// reached touch a file. That marker is gone. What the test reads now is the
// journal entry itself, which is what the system is for rather than a stand in
// for it.
//
// What the two hops carry differs, and the difference is the point. The socket
// request names the workspace and presents a credential, because the socket is
// local IPC that authenticates nobody. The coderd request carries nothing:
// coderd resolved the workspace, its owner, and this workspace_agent while
// authenticating the connection. So the entry's contents were stated by nobody.
// They are what coderd concluded.
//
// The test passes on two independent conditions: the executable exited zero,
// observed through the script timings the agent reports, and the journal holds
// one creation entry attributed to the workspace_agent.
//
// The manual control that earlier revisions needed is gone with the marker. It
// existed because a missing file could not be told apart from a script that
// never ran. A query for entries returns an empty set either way, and the test
// requires exactly one, so the check now runs automatically.
func TestAIAgentIdentity(t *testing.T) {
	t.Parallel()

	t.Run("ProbeReachesAgentOverSocket", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		probeBin := buildProbe(t)
		socketPath := testutil.AgentSocketPath(t)

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
		// The remaining two have no production source. Nothing exports the
		// socket path today, and the binary path exists only for this test.
		// Agent-level variables are the mechanism the agent itself uses, and
		// they take precedence over everything else.
		_ = agenttest.New(t, client.URL, r.AgentToken, func(o *agent.Options) {
			// agenttest sets a socket path but leaves the server disabled, so
			// without this there is nothing listening for the probe to dial.
			o.SocketServerEnabled = true
			o.SocketPath = socketPath
			o.LogDir = logDir
			o.EnvironmentVariables = map[string]string{
				"CODER_AGENT_SOCKET_PATH": socketPath,
				"CODER_POC_PROBE_BIN":     probeBin,
			}
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).AgentNames([]string{}).Wait()

		// Reading the journal needs system permission, which is the same
		// reason the workspace_agent cannot write to it. See the handler in
		// coderd/agentapi/aiagent.go.
		//nolint:gocritic // Reading the journal and the agent's timings needs a system context.
		systemCtx := dbauthz.AsSystemRestricted(ctx)

		// Exit status is checked first and independently of the database. The
		// agent reports it, so a probe whose call succeeded and which then
		// failed would still be caught, and waiting on it means the script log
		// read below is complete.
		require.Len(t, r.Agents, 1, "the fake build should have exactly one agent")
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

		// The identity is minted by coderd and reaches the test only by the
		// route a real caller would use: returned over the socket, printed by
		// the probe. Taking it from there rather than from the database means
		// the value handed back to the caller is the value checked below, so
		// a handler that persisted one identity and returned another would be
		// caught.
		scriptLog := readScriptLog(scriptLogPath)
		id := mintedAIAgentID(t, scriptLog)

		aiAgent, err := db.GetAIAgentLifecycleLedgerRowByID(systemCtx, id)
		require.NoError(t, err, "the returned identity should name a row")
		require.Equal(t, string(entity.TypeUser), aiAgent.OwnerType)
		require.Equal(t, user.UserID, aiAgent.OwnerID,
			"the AI agent should belong to the owner of the workspace it was created in")
		require.Equal(t, entity.AIAgentStateActive, aiAgent.State)

		// The journal accounts for that row. Until the AI agent table landed
		// every entry pointed at nothing, so this is what makes the journal a
		// record of the world rather than of itself.
		entries, err := db.GetAIAgentLifecycleEntriesBySubject(systemCtx, database.GetAIAgentLifecycleEntriesBySubjectParams{
			Subject: id,
			Limit:   10,
		})
		require.NoError(t, err)
		require.Len(t, entries, 1, "one creation should produce one entry")

		got := entries[0]
		require.Equal(t, string(entity.EventAIAgentCreate), got.Event)
		require.Equal(t, string(entity.TypeUser), got.ActorType.String,
			"creation is commanded by the owner, not by the workspace_agent that relayed it")
		require.Equal(t, user.UserID, got.Actor.UUID)
		require.True(t, got.RecordingDate.Valid)
		require.Equal(t, got.EntryID, aiAgent.PostingReference,
			"the ledger row should name the entry that posted to it")

		// The grant. Creation confers authority as well as identity, and the
		// authorization is the only one of the three entities whose identifier
		// never leaves coderd, so it is reached through the agent it was
		// granted over rather than by identifier.
		grants, err := db.GetAuthorizationLifecycleLedgerRowsByAgent(systemCtx, database.GetAuthorizationLifecycleLedgerRowsByAgentParams{
			AgentType: string(entity.TypeAIAgent),
			AgentID:   id,
		})
		require.NoError(t, err)
		require.Len(t, grants, 1, "creation should confer exactly one authorization")

		grant := grants[0]
		require.Equal(t, string(entity.TypeUser), grant.PrincipalType)
		require.Equal(t, user.UserID, grant.PrincipalID,
			"the principal is the owner, whose order brought the agent about")
		require.Equal(t, entity.UniversalScope, grant.Scope, "the proof of concept grants universally")
		require.Equal(t, entity.StateActive, grant.State)

		grantEntries, err := db.GetAuthorizationLifecycleJournalEntriesBySubject(systemCtx, database.GetAuthorizationLifecycleJournalEntriesBySubjectParams{
			Subject: grant.ID,
			Limit:   10,
		})
		require.NoError(t, err)
		require.Len(t, grantEntries, 1, "a grant is one entry")

		grantEntry := grantEntries[0]
		require.Equal(t, string(entity.EventGrant), grantEntry.Event)
		require.Equal(t, string(entity.TypeUser), grantEntry.ActorType.String,
			"a grant is an act of the principal")
		require.Equal(t, user.UserID, grantEntry.Actor.UUID)
		require.Equal(t, grantEntry.EntryID, grant.PostingReference,
			"the ledger row should name the entry that posted to it")

		// The credential reached the executable. It is compared by digest
		// because the executable does not print it: standard output becomes a
		// log the control plane stores, and a credential does not belong there.
		credentials, err := db.GetValidCredentialsByHolder(systemCtx, database.GetValidCredentialsByHolderParams{
			HolderType: string(entity.TypeAIAgent),
			HolderID:   id,
		})
		require.NoError(t, err)
		require.Len(t, credentials, 1, "creation should issue exactly one credential")

		// The digest is on the password type's own row, which the ledger row
		// names by carrying its type. The comparison is then direct: what is on
		// record against what the executable received, hashed the same way.
		require.Equal(t, entity.CredentialTypePassword, credentials[0].CredentialType)
		password, err := db.GetCredentialPasswordByID(systemCtx, credentials[0].ID)
		require.NoError(t, err)
		require.Equal(t, password.HashedAuthenticator, reportedCredentialDigest(t, scriptLog),
			"the authenticator the executable received should be the one on record")
	})

	// The probe is what a real workspace will run, so its failure handling is
	// behavior in its own right and not only a check on this test. These run
	// it directly rather than through a workspace: what is under test is the
	// probe's own handling of a broken environment, and standing up an agent
	// would only slow that down.

	t.Run("ProbeFailsWhenSocketIsAbsent", func(t *testing.T) {
		t.Parallel()

		absentSocket := filepath.Join(t.TempDir(), "nothing-listening.sock")

		out, err := runProbe(t, map[string]string{
			"CODER_AGENT_SOCKET_PATH": absentSocket,
			"CODER_WORKSPACE_ID":      uuid.NewString(),
			"CODER_AGENT_TOKEN":       uuid.NewString(),
		})

		require.Error(t, err, "probe should fail when nothing is listening on the socket")
		require.Contains(t, out, "connect to agent socket",
			"failure should name the socket rather than fail obscurely")
	})

	t.Run("ProbeFailsWhenAVariableIsMissing", func(t *testing.T) {
		t.Parallel()

		// Each required variable is dropped in turn. A probe that silently
		// defaulted any of them would pass its own happy path while giving the
		// eventual create-agent call the wrong workspace or credential.
		required := []string{
			"CODER_AGENT_SOCKET_PATH",
			"CODER_WORKSPACE_ID",
			"CODER_AGENT_TOKEN",
		}

		for _, missing := range required {
			t.Run(missing, func(t *testing.T) {
				t.Parallel()

				env := map[string]string{
					"CODER_AGENT_SOCKET_PATH": filepath.Join(t.TempDir(), "unused.sock"),
					"CODER_WORKSPACE_ID":      uuid.NewString(),
					"CODER_AGENT_TOKEN":       uuid.NewString(),
				}
				delete(env, missing)

				out, err := runProbe(t, env)

				require.Error(t, err, "probe should fail when %s is missing", missing)
				require.Contains(t, out, missing+" is not set",
					"failure should name the missing variable")
			})
		}
	})
}

// mintedAIAgentID extracts the identity the probe reported from its output.
//
// The probe prints one line naming what coderd minted for it. Reading it back
// out of the log is how the test learns an identity it never chose and cannot
// otherwise know.
func mintedAIAgentID(t *testing.T, scriptLog string) uuid.UUID {
	t.Helper()

	matches := regexp.MustCompile(`created AI agent ([0-9a-fA-F-]{36})`).FindStringSubmatch(scriptLog)
	require.Len(t, matches, 2, "probe did not report an AI agent identity.\nscript log:\n%s", scriptLog)

	id, err := uuid.Parse(matches[1])
	require.NoError(t, err)
	return id
}

// reportedCredentialDigest extracts the digest of the credential the probe
// received. The probe reports a digest rather than the credential because its
// output is stored as a log by the control plane.
func reportedCredentialDigest(t *testing.T, scriptLog string) string {
	t.Helper()

	matches := regexp.MustCompile(`credential sha256 ([0-9a-f]{64})`).FindStringSubmatch(scriptLog)
	require.Len(t, matches, 2, "probe did not report a credential digest.\nscript log:\n%s", scriptLog)
	return matches[1]
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
