package workspaceexec

import (
	"context"
	"database/sql"
	"encoding/hex"
	"math"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Observe reads the original epoch's process without starting or retrying it.
// A receipt may retain a known exit even when ephemeral output has expired.
func Observe(ctx context.Context, db database.Store, agent ProcessAgent, receipt database.WorkspaceExecutionReceipt, waitMillis int64, now time.Time) (database.WorkspaceExecutionReceipt, workspacesdk.ProcessOutputResponse, error) {
	output, observeErr := agent.ProcessOutput(ctx, receipt.ProcessID.String(), &workspacesdk.ProcessOutputOptions{
		Wait: waitMillis > 0, WaitMillis: waitMillis, AgentInstanceID: receipt.AgentInstanceID, InputDigest: hex.EncodeToString(receipt.InputDigest),
	})
	if observeErr != nil {
		state := "unknown"
		var code *int
		var expired *workspacesdk.ProcessOutputUnavailableError
		if xerrors.As(observeErr, &expired) && expired.AgentInstanceID == receipt.AgentInstanceID &&
			expired.InputDigest == hex.EncodeToString(receipt.InputDigest) && expired.Process.ID == receipt.ProcessID.String() &&
			expired.OutputUnavailable && !expired.Process.Running && expired.Process.ExitCode != nil && expired.Process.ExitedAt != nil {
			state, code = "completed", expired.Process.ExitCode
		}
		updated, err := recordObservation(ctx, db, receipt, state, code, observeErr.Error(), now)
		if err != nil {
			return receipt, output, err
		}
		return updated, output, observeErr
	}
	state := "running"
	if !output.Running {
		if output.ExitCode == nil {
			return receipt, output, xerrors.New("agent omitted terminal exit code")
		}
		state = "completed"
	} else if output.ExitCode != nil {
		return receipt, output, xerrors.New("agent reported an exit while process is running")
	}
	updated, err := recordObservation(ctx, db, receipt, state, output.ExitCode, "", now)
	if err == nil && output.Running && (updated.State == "completed" || updated.State == "not_started") {
		return updated, workspacesdk.ProcessOutputResponse{}, xerrors.New("output snapshot predates terminal receipt; observe again for final output")
	}
	return updated, output, err
}

// Cancel fences an absent identity or records actual terminal acknowledgment.
// Sending cancellation or reaching a deadline never manufactures completion.
func Cancel(ctx context.Context, db database.Store, agent ProcessAgent, receipt database.WorkspaceExecutionReceipt, waitMillis int64, now time.Time) (database.WorkspaceExecutionReceipt, error) {
	if receipt.State == "completed" || receipt.State == "not_started" {
		return receipt, nil
	}
	result, cancelErr := agent.CancelProcess(ctx, workspacesdk.CancelProcessRequest{
		ProcessID: receipt.ProcessID, AgentInstanceID: receipt.AgentInstanceID,
		InputDigest: hex.EncodeToString(receipt.InputDigest), Deadline: receipt.Deadline.Time, WaitMillis: waitMillis,
	})
	if cancelErr != nil {
		return recordObservation(ctx, db, receipt, "unknown", nil, cancelErr.Error(), now)
	}
	if result.AgentInstanceID != receipt.AgentInstanceID {
		return recordObservation(ctx, db, receipt, "unknown", nil, "agent epoch changed during cancellation", now)
	}
	if result.Fenced {
		if result.Process != nil {
			return receipt, xerrors.New("agent returned both process and absent-identity fence")
		}
		return recordObservation(ctx, db, receipt, "not_started", nil, "", now)
	}
	if result.Process == nil {
		return recordObservation(ctx, db, receipt, "unknown", nil, "process history expired", now)
	}
	if result.Process.ID != receipt.ProcessID.String() {
		return receipt, xerrors.New("agent cancellation returned another process")
	}
	if result.Process.Running {
		if result.Process.ExitCode != nil {
			return receipt, xerrors.New("agent cancellation reported a running exit")
		}
		return recordObservation(ctx, db, receipt, "running", nil, "cancellation not yet acknowledged", now)
	}
	if result.Process.ExitCode == nil {
		return receipt, xerrors.New("agent cancellation omitted terminal exit")
	}
	return recordObservation(ctx, db, receipt, "completed", result.Process.ExitCode, "", now)
}

func recordObservation(ctx context.Context, db database.Store, receipt database.WorkspaceExecutionReceipt, state string, exitCode *int, detail string, now time.Time) (database.WorkspaceExecutionReceipt, error) {
	code := sql.NullInt32{}
	if exitCode != nil {
		if *exitCode < math.MinInt32 || *exitCode > math.MaxInt32 {
			return receipt, xerrors.New("agent exit code is out of range")
		}
		code = sql.NullInt32{Int32: int32(*exitCode), Valid: true}
	}
	updated, err := db.UpdateWorkspaceExecutionReceipt(ctx, database.UpdateWorkspaceExecutionReceiptParams{
		ID: receipt.ID, State: state, ExitCode: code, Error: detail, UpdatedAt: dbtime.Time(now),
	})
	if xerrors.Is(err, sql.ErrNoRows) {
		updated, err = db.GetWorkspaceExecutionReceiptByID(ctx, receipt.ID)
	}
	if err != nil {
		return receipt, xerrors.Errorf("persist process observation: %w", err)
	}
	return updated, nil
}
