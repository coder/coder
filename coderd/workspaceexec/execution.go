package workspaceexec

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ErrRequestConflict means a durable request identity has different input.
var ErrRequestConflict = xerrors.New("execution request identity has different input")

// ErrUnsupportedAgent means the agent cannot safely acknowledge tracked starts.
var ErrUnsupportedAgent = xerrors.New("workspace agent does not support tracked execution")

// ProcessAgent is the narrow process protocol needed by durable execution.
type ProcessAgent interface {
	ListProcesses(context.Context) (workspacesdk.ListProcessesResponse, error)
	StartProcess(context.Context, workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error)
	ProcessOutput(context.Context, string, *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error)
	CancelProcess(context.Context, workspacesdk.CancelProcessRequest) (workspacesdk.CancelProcessResponse, error)
}

// StartInput carries live input only. Commands and environments are not persisted.
type StartInput struct {
	SessionID, ActorID, RequestID, AgentID uuid.UUID
	Command, WorkDir                       string
	Env                                    map[string]string
}

// Start records the planned process and agent epoch before exactly one dispatch.
// Existing receipts never cause an automatic re-send, including after restart.
func Start(ctx context.Context, db database.Store, agent ProcessAgent, input StartInput, now time.Time) (database.WorkspaceExecutionReceipt, error) {
	session, err := db.GetWorkspaceExecutionSessionByID(ctx, input.SessionID)
	if err != nil {
		return database.WorkspaceExecutionReceipt{}, err
	}
	var declaration struct {
		ExecutionDeadline *time.Time `json:"execution_deadline"`
	}
	if err := json.Unmarshal(session.Declarations, &declaration); err != nil {
		return database.WorkspaceExecutionReceipt{}, xerrors.Errorf("decode execution declaration: %w", err)
	}
	request := workspacesdk.StartProcessRequest{Command: input.Command, WorkDir: input.WorkDir, Env: input.Env, Background: true}
	if declaration.ExecutionDeadline != nil {
		// Hash, persist, and dispatch the same Postgres-compatible deadline.
		request.Deadline = dbtime.Time(declaration.ExecutionDeadline.UTC())
	}
	request.InputDigest = workspacesdk.StartProcessDigest(request)
	digest, err := hex.DecodeString(request.InputDigest)
	if err != nil {
		return database.WorkspaceExecutionReceipt{}, err
	}
	key := database.GetWorkspaceExecutionReceiptByRequestParams{SessionID: input.SessionID, ActorID: input.ActorID, RequestID: input.RequestID}
	check := func(receipt database.WorkspaceExecutionReceipt) (database.WorkspaceExecutionReceipt, error) {
		if receipt.AgentID != input.AgentID || !bytes.Equal(receipt.InputDigest, digest) {
			return receipt, ErrRequestConflict
		}
		return receipt, nil
	}
	if prior, err := db.GetWorkspaceExecutionReceiptByRequest(ctx, key); err == nil {
		return check(prior)
	} else if !xerrors.Is(err, sql.ErrNoRows) {
		return prior, err
	}
	if input.RequestID == uuid.Nil || input.Command == "" || !session.WorkspaceID.Valid {
		return database.WorkspaceExecutionReceipt{}, xerrors.New("request, command, and acquired workspace are required")
	}
	if agent == nil {
		return database.WorkspaceExecutionReceipt{}, xerrors.New("original execution receipt no longer exists")
	}
	capabilities, err := agent.ListProcesses(ctx)
	if err != nil {
		return database.WorkspaceExecutionReceipt{}, xerrors.Errorf("discover process capability: %w", err)
	}
	if capabilities.ProtocolVersion < workspacesdk.TrackedProcessProtocolVersion || capabilities.AgentInstanceID == uuid.Nil {
		return database.WorkspaceExecutionReceipt{}, ErrUnsupportedAgent
	}
	request.AgentInstanceID = capabilities.AgentInstanceID
	request.ProcessID = uuid.New()
	var receipt database.WorkspaceExecutionReceipt
	fresh := false
	err = db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, database.WorkspaceLifecycleLockID(session.WorkspaceID.UUID)); err != nil {
			return err
		}
		prior, err := tx.GetWorkspaceExecutionReceiptByRequest(ctx, key)
		if err == nil {
			receipt, err = check(prior)
			return err
		}
		if !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := CheckAdmission(ctx, tx, session.WorkspaceID.UUID); err != nil {
			return err
		}
		current, err := tx.GetWorkspaceExecutionSessionByID(ctx, input.SessionID)
		if err != nil {
			return err
		}
		workspace, err := tx.GetWorkspaceByID(ctx, current.WorkspaceID.UUID)
		if err != nil {
			return err
		}
		if current.WorkspaceID != session.WorkspaceID || workspace.OwnerID != current.WorkspaceOwnerID.UUID || workspace.OrganizationID != current.OrganizationID {
			return ErrAdmissionClosed
		}
		receipt, err = tx.InsertWorkspaceExecutionReceipt(ctx, database.InsertWorkspaceExecutionReceiptParams{
			ID: uuid.New(), SessionID: input.SessionID, ActorID: input.ActorID, RequestID: input.RequestID, InputDigest: digest,
			AgentID: input.AgentID, AgentInstanceID: request.AgentInstanceID, ProcessID: request.ProcessID,
			AdmissionRevision: current.Revision, CreatedAt: dbtime.Time(now),
			Deadline: sql.NullTime{Time: request.Deadline, Valid: !request.Deadline.IsZero()},
		})
		fresh = err == nil
		return err
	}, nil)
	if err != nil {
		return receipt, err
	}
	if !fresh {
		return receipt, nil
	}
	// Client disappearance must not cancel admitted dispatch or its final receipt.
	dispatchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	started, dispatchErr := agent.StartProcess(dispatchCtx, request)
	state, detail := "running", ""
	if dispatchErr != nil {
		state, detail = "unknown", dispatchErr.Error()
	} else if started.ID != request.ProcessID.String() {
		state, detail = "unknown", "agent returned a different process identity"
	}
	updated, err := recordObservation(dispatchCtx, db, receipt, state, nil, detail, now)
	if err != nil {
		return receipt, xerrors.Errorf("execution %s was admitted; receipt update is uncertain: %w", receipt.ID, err)
	}
	return updated, nil
}
