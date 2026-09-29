package workspaceexec_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

type executionAgent struct {
	workspaceexec.ProcessAgent
	epoch  uuid.UUID
	start  func(context.Context, workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error)
	output func(context.Context, string, *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error)
	cancel func(context.Context, workspacesdk.CancelProcessRequest) (workspacesdk.CancelProcessResponse, error)
}

func (a *executionAgent) ListProcesses(context.Context) (workspacesdk.ListProcessesResponse, error) {
	return workspacesdk.ListProcessesResponse{ProtocolVersion: 1, AgentInstanceID: a.epoch}, nil
}

func (a *executionAgent) StartProcess(ctx context.Context, r workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
	return a.start(ctx, r)
}

func (a *executionAgent) ProcessOutput(ctx context.Context, id string, o *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
	return a.output(ctx, id, o)
}

func (a *executionAgent) CancelProcess(ctx context.Context, r workspacesdk.CancelProcessRequest) (workspacesdk.CancelProcessResponse, error) {
	return a.cancel(ctx, r)
}

func executionFixture(t *testing.T) (database.Store, workspaceexec.StartInput, time.Time) {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: user.ID, TemplateID: template.ID})
	now := dbtime.Now()
	row, err := db.InsertWorkspaceExecutionSession(t.Context(), database.InsertWorkspaceExecutionSessionParams{
		ID: uuid.New(), OrganizationID: org.ID, OwnerID: user.ID, ActorID: user.ID, RequestID: uuid.New(), InputDigest: make([]byte, 32),
		WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true}, WorkspaceOwnerID: uuid.NullUUID{UUID: user.ID, Valid: true}, CreatedAt: now,
		State: "active", Disposable: true, LeaseExpiresAt: now.Add(time.Hour), Declarations: []byte("{}"),
	})
	require.NoError(t, err)
	return db, workspaceexec.StartInput{SessionID: row.ID, ActorID: user.ID, RequestID: uuid.New(), AgentID: uuid.New(), Command: "side effect"}, now
}

func TestExecutionLostDispatchResponseNeverReplays(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, input, now := executionFixture(t)
	agent := &executionAgent{epoch: uuid.New()}
	calls := 0
	agent.start = func(ctx context.Context, request workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
		calls++
		stored, err := db.GetWorkspaceExecutionReceiptByRequest(ctx, database.GetWorkspaceExecutionReceiptByRequestParams{SessionID: input.SessionID, ActorID: input.ActorID, RequestID: input.RequestID})
		require.NoError(t, err)
		require.Equal(t, "dispatching", stored.State)
		require.Equal(t, request.ProcessID, stored.ProcessID)
		require.Equal(t, agent.epoch, stored.AgentInstanceID)
		return workspacesdk.StartProcessResponse{}, io.ErrUnexpectedEOF
	}
	receipt, err := workspaceexec.Start(ctx, db, agent, input, now)
	require.NoError(t, err)
	require.Equal(t, "unknown", receipt.State)
	require.False(t, receipt.ExitCode.Valid)
	// A new caller needs no live agent, even after its lease has expired.
	replay, err := workspaceexec.Start(ctx, db, nil, input, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.Equal(t, receipt.ID, replay.ID)
	require.Equal(t, 1, calls)
	changed := input
	changed.Command = "different"
	_, err = workspaceexec.Start(ctx, db, nil, changed, now)
	require.ErrorIs(t, err, workspaceexec.ErrRequestConflict)
	// The original epoch is mandatory on recovery; an unavailable/restarted agent
	// must never manufacture either a fresh execution or a terminal exit.
	agent.output = func(_ context.Context, id string, opts *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
		require.Equal(t, receipt.ProcessID.String(), id)
		require.Equal(t, agent.epoch, opts.AgentInstanceID)
		return workspacesdk.ProcessOutputResponse{}, io.ErrUnexpectedEOF
	}
	observed, _, err := workspaceexec.Observe(ctx, db, agent, receipt, 0, now)
	require.Error(t, err)
	require.Equal(t, "unknown", observed.State)
	require.False(t, observed.ExitCode.Valid)
	agent.cancel = func(_ context.Context, request workspacesdk.CancelProcessRequest) (workspacesdk.CancelProcessResponse, error) {
		require.Equal(t, receipt.ProcessID, request.ProcessID)
		require.Equal(t, agent.epoch, request.AgentInstanceID)
		return workspacesdk.CancelProcessResponse{AgentInstanceID: agent.epoch, Fenced: true}, nil
	}
	fenced, err := workspaceexec.Cancel(ctx, db, agent, observed, 0, now)
	require.NoError(t, err)
	require.Equal(t, "not_started", fenced.State)
	require.False(t, fenced.ExitCode.Valid)
	_, err = workspaceexec.Start(ctx, db, nil, input, now)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}
