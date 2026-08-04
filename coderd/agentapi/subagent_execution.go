package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/quartz"
)

// maxSubagentExecutionErrorBytes mirrors the octet_length limit the
// workspace_agent_subagent_execution_statuses.last_error check constraint
// enforces. Reports above it are rejected before the database is touched, and
// never truncated, so a launcher cannot silently record a different error than
// the one it observed.
const maxSubagentExecutionErrorBytes = 4096

// SubagentExecutionAPI serves the control plane for Coder-owned nested
// executions. Only the authenticated top-level parent agent of a declaration may
// acquire it or report on it, and the child's auth token leaves coderd through
// AcquireSubagentExecution alone.
type SubagentExecutionAPI struct {
	AgentFn func(context.Context) (database.WorkspaceAgent, error)

	Log      slog.Logger
	Clock    quartz.Clock
	Database database.Store
}

var (
	// errSubagentExecutionUnavailable is the single answer to every failure that
	// depends on the execution tuple: a declaration that does not exist, one
	// owned by another parent, a superseded generation, a fenced launcher, a
	// refused transition, or a child that is gone. Reporting them apart would let
	// a parent probe for declarations and children it does not own.
	errSubagentExecutionUnavailable = xerrors.New("subagent execution is unavailable")
	// errNestedSubagentExecutionsUnsupported rejects a child agent before any
	// database call, so a nested agent never reaches the acquisition path.
	errNestedSubagentExecutionsUnsupported = xerrors.New("child agents cannot control subagent executions")
	// errSubagentExecutionStatusNotReportable rejects statuses a launcher may
	// never report: the unspecified value, 'starting', which the acquisition
	// owns, and any value this coderd does not know.
	errSubagentExecutionStatusNotReportable = xerrors.New("subagent execution status is not reportable")
	// errSubagentExecutionAcquisitionRequired rejects a report that carries no
	// acquisition version, which no launcher of an acquired declaration has.
	errSubagentExecutionAcquisitionRequired = xerrors.New("subagent execution acquisition version is required")
	// errSubagentExecutionErrorTooLong rejects an oversized error string instead
	// of truncating it or letting the database's check violation surface.
	errSubagentExecutionErrorTooLong = xerrors.New("subagent execution error is too long")
)

// authenticatedParent resolves the calling agent and requires it to be
// top-level. A child agent is rejected here, before any database call, so it can
// neither acquire credentials nor report on its parent's declarations.
func (a *SubagentExecutionAPI) authenticatedParent(ctx context.Context) (database.WorkspaceAgent, error) {
	parentAgent, err := a.AgentFn(ctx)
	if err != nil {
		return database.WorkspaceAgent{}, xerrors.Errorf("get parent agent: %w", err)
	}
	if parentAgent.ParentID.Valid {
		return database.WorkspaceAgent{}, errNestedSubagentExecutionsUnsupported
	}
	return parentAgent, nil
}

func (a *SubagentExecutionAPI) now() time.Time {
	clock := a.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	return dbtime.Time(clock.Now())
}

// subagentExecutionTuple is the (generation, declaration) pair a request claims
// to own. The parent agent ID is never taken from the request: it is always the
// authenticated caller.
type subagentExecutionTuple struct {
	workspaceBuildID uuid.UUID
	declarationID    uuid.UUID
}

// parseSubagentExecutionTuple decodes the request's opaque ID bytes. Malformed
// bytes are answered with the same sentinel as an unknown declaration, because
// the caller learns nothing either way.
func parseSubagentExecutionTuple(executionID, generation []byte) (subagentExecutionTuple, error) {
	declarationID, err := uuid.FromBytes(executionID)
	if err != nil {
		return subagentExecutionTuple{}, errSubagentExecutionUnavailable
	}
	workspaceBuildID, err := uuid.FromBytes(generation)
	if err != nil {
		return subagentExecutionTuple{}, errSubagentExecutionUnavailable
	}
	return subagentExecutionTuple{
		workspaceBuildID: workspaceBuildID,
		declarationID:    declarationID,
	}, nil
}

// subagentExecutionStatusFromProto maps a launcher-observed status onto the
// database's status text. 'pending' has no proto value because it exists before
// any launcher, and 'starting' is deliberately unmapped: only an acquisition may
// enter it, which is what bumps the fencing acquisition version.
func subagentExecutionStatusFromProto(status agentproto.ReportSubagentExecutionStatusRequest_Status) (database.SubagentExecutionStatus, error) {
	switch status {
	case agentproto.ReportSubagentExecutionStatusRequest_RUNNING:
		return database.SubagentExecutionStatusRunning, nil
	case agentproto.ReportSubagentExecutionStatusRequest_STOPPING:
		return database.SubagentExecutionStatusStopping, nil
	case agentproto.ReportSubagentExecutionStatusRequest_STOPPED:
		return database.SubagentExecutionStatusStopped, nil
	case agentproto.ReportSubagentExecutionStatusRequest_FAILED:
		return database.SubagentExecutionStatusFailed, nil
	default:
		return "", errSubagentExecutionStatusNotReportable
	}
}

// isSubagentExecutionInaccessible reports whether the database refused the tuple
// in a way the caller must not be able to tell apart. Every validation failure
// inside the acquisition and the report surfaces as sql.ErrNoRows, and dbauthz
// refuses a parent whose workspace the actor cannot reach. A check violation is
// included because an unexpected value must not leak the constraint that caught
// it.
func isSubagentExecutionInaccessible(err error) bool {
	return errors.Is(err, sql.ErrNoRows) ||
		dbauthz.IsNotAuthorizedError(err) ||
		database.IsCheckViolation(err)
}

// AcquireSubagentExecution hands the authenticated parent agent the pre-created
// child credentials for one declared execution, fencing every earlier launcher
// of that declaration.
//
// The parent agent ID is the authenticated caller's, never a request field, so a
// parent can only acquire its own declarations. The response is the only place
// the child auth token is returned: the manifest carries declarations without
// credentials.
func (a *SubagentExecutionAPI) AcquireSubagentExecution(ctx context.Context, req *agentproto.AcquireSubagentExecutionRequest) (*agentproto.AcquireSubagentExecutionResponse, error) {
	if req == nil {
		return nil, errSubagentExecutionUnavailable
	}
	tuple, err := parseSubagentExecutionTuple(req.ExecutionId, req.Generation)
	if err != nil {
		return nil, err
	}

	parent, err := a.authenticatedParent(ctx)
	if err != nil {
		return nil, err
	}

	// The acquisition takes the workspace build publication advisory lock for the
	// duration of its own transaction, so it must be called standalone rather
	// than from an outer transaction that would hold the guard far longer.
	acquired, err := a.Database.AcquireWorkspaceAgentSubagentExecution(ctx, database.AcquireWorkspaceAgentSubagentExecutionParams{
		WorkspaceBuildID: tuple.workspaceBuildID,
		DeclarationID:    tuple.declarationID,
		ParentAgentID:    parent.ID,
		Now:              a.now(),
	})
	if err != nil {
		if isSubagentExecutionInaccessible(err) {
			a.Log.Debug(ctx, "subagent execution acquisition refused",
				slog.F("parent_agent_id", parent.ID),
				slog.F("declaration_id", tuple.declarationID),
				slog.Error(err))
			return nil, errSubagentExecutionUnavailable
		}
		return nil, xerrors.Errorf("acquire subagent execution: %w", err)
	}

	return &agentproto.AcquireSubagentExecutionResponse{
		ChildAgentId:       acquired.ChildAgentID[:],
		AuthToken:          acquired.AuthToken.String(),
		AcquisitionVersion: acquired.AcquisitionVersion,
	}, nil
}

// ReportSubagentExecutionStatus records the launcher-observed state of one
// declared execution the authenticated parent previously acquired.
//
// The acquisition version fences a launcher that has been replaced, and the
// database decides which transitions are permitted. Every state-dependent
// refusal is reported as the same sentinel as an unknown declaration.
func (a *SubagentExecutionAPI) ReportSubagentExecutionStatus(ctx context.Context, req *agentproto.ReportSubagentExecutionStatusRequest) (*agentproto.ReportSubagentExecutionStatusResponse, error) {
	if req == nil {
		return nil, errSubagentExecutionUnavailable
	}
	tuple, err := parseSubagentExecutionTuple(req.ExecutionId, req.Generation)
	if err != nil {
		return nil, err
	}
	// A launcher that never acquired has no version to echo, and the database
	// keeps such a declaration pending. Reject it here so the pending state is
	// not probed through the report path.
	if req.AcquisitionVersion <= 0 {
		return nil, errSubagentExecutionAcquisitionRequired
	}
	status, err := subagentExecutionStatusFromProto(req.Status)
	if err != nil {
		return nil, err
	}
	// The column is bounded by octet length, so the limit is applied to the UTF-8
	// bytes rather than the rune count. Invalid UTF-8 is rejected too: the column
	// is text, so the database would refuse it with an encoding error.
	if len(req.Error) > maxSubagentExecutionErrorBytes {
		return nil, errSubagentExecutionErrorTooLong
	}
	if !utf8.ValidString(req.Error) {
		return nil, errSubagentExecutionUnavailable
	}

	parent, err := a.authenticatedParent(ctx)
	if err != nil {
		return nil, err
	}

	// Called standalone for the same reason as the acquisition: it takes the
	// workspace build publication guard inside its own transaction.
	if _, err := a.Database.ReportWorkspaceAgentSubagentExecutionStatus(ctx, database.ReportWorkspaceAgentSubagentExecutionStatusParams{
		WorkspaceBuildID:   tuple.workspaceBuildID,
		DeclarationID:      tuple.declarationID,
		ParentAgentID:      parent.ID,
		AcquisitionVersion: req.AcquisitionVersion,
		Status:             status,
		LastError:          req.Error,
		Now:                a.now(),
	}); err != nil {
		if isSubagentExecutionInaccessible(err) {
			a.Log.Debug(ctx, "subagent execution report refused",
				slog.F("parent_agent_id", parent.ID),
				slog.F("declaration_id", tuple.declarationID),
				slog.F("status", string(status)),
				slog.Error(err))
			return nil, errSubagentExecutionUnavailable
		}
		return nil, xerrors.Errorf("report subagent execution status: %w", err)
	}

	return &agentproto.ReportSubagentExecutionStatusResponse{}, nil
}
