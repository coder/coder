// Package subagentexec manages the nested executions a parent agent's
// manifest declares.
//
// The manifest carries declarations only. The child agent's ID, auth
// token, and fencing acquisition version are fetched separately through
// AcquireSubagentExecution, so credentials never reach an agent that does
// not launch the execution. This package owns that acquisition, hands the
// resulting launch to a driver, and reports the launcher-observed status
// back to coderd.
package subagentexec

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// MaxReportErrorBytes bounds the error string attached to a status
// report. It mirrors the limit coderd enforces on the persisted error, so
// a report is never rejected for length alone.
const MaxReportErrorBytes = 4096

// ErrDriverNotConfigured is returned by the default driver. An agent that
// declares no executions never launches anything, so a deployment without
// a configured driver behaves exactly as it did before.
var ErrDriverNotConfigured = xerrors.New("subagent execution driver is not configured")

// State is the launcher-observed state of a declared execution as the
// manager last saw it. It intentionally omits the 'pending' state, which
// exists in coderd before any launcher reports.
type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateFailed   State = "failed"
)

// Controller is the subset of the v2.11 Agent API the manager needs.
// proto.DRPCAgentClient211 satisfies it. It is nil when the agent fell
// back to an older API version, in which case no declaration is launched.
type Controller interface {
	AcquireSubagentExecution(ctx context.Context, in *proto.AcquireSubagentExecutionRequest) (*proto.AcquireSubagentExecutionResponse, error)
	ReportSubagentExecutionStatus(ctx context.Context, in *proto.ReportSubagentExecutionStatusRequest) (*proto.ReportSubagentExecutionStatusResponse, error)
}

// Driver launches one acquired execution. Implementations must return
// promptly: the manager supervises the returned Process instead of
// blocking the reconciliation loop on the run itself.
type Driver interface {
	Start(ctx context.Context, launch Launch) (Process, error)
}

// Process is a launched execution the manager supervises. Wait blocks
// until the run ends. Stop asks the run to end and may be called
// concurrently with Wait.
type Process interface {
	Wait() error
	Stop(ctx context.Context) error
}

// Launch is everything a driver needs to start one acquired execution.
//
// The child's auth token is deliberately unexported: only this package
// can read it, so an out-of-tree driver cannot copy it into a log line, an
// argument list, or an environment variable by accident. The in-package
// ScriptDriver reads it exactly once, to write the private 0600 token file
// the sandboxed child reads through CODER_AGENT_TOKEN_FILE, and the
// manager drops its own reference as soon as Start returns.
type Launch struct {
	// Declaration is the manifest declaration being launched.
	Declaration agentsdk.SubagentExecution
	// ChildAgentID is the pre-created child agent this launch runs as.
	ChildAgentID uuid.UUID
	// AcquisitionVersion fences this launch against a superseded one.
	AcquisitionVersion int64

	// authToken is the pre-created child agent's auth token.
	authToken string
}

// String redacts the launch so that formatting it, directly or through a
// log field, cannot leak the child's auth token.
func (l Launch) String() string {
	return fmt.Sprintf("subagentexec.Launch{ExecutionID:%s Generation:%s Name:%q Driver:%q ChildAgentID:%s AcquisitionVersion:%d}",
		l.Declaration.ExecutionID, l.Declaration.Generation, l.Declaration.Name,
		l.Declaration.Driver, l.ChildAgentID, l.AcquisitionVersion)
}

// Status is the redacted, externally visible view of one declared
// execution. It never carries the child's auth token.
type Status struct {
	ExecutionID        uuid.UUID `json:"execution_id"`
	Generation         uuid.UUID `json:"generation"`
	Name               string    `json:"name"`
	Driver             string    `json:"driver"`
	ChildAgentID       uuid.UUID `json:"child_agent_id"`
	AcquisitionVersion int64     `json:"acquisition_version"`
	State              State     `json:"state"`
	// LastError is the bounded error from the most recent failure, empty
	// when the execution has not failed.
	LastError string `json:"last_error,omitempty"`
}

// BoundError truncates s to MaxReportErrorBytes without splitting a
// rune, so an oversized driver error is still reportable.
func BoundError(s string) string {
	if len(s) <= MaxReportErrorBytes {
		return s
	}
	bounded := s[:MaxReportErrorBytes]
	for len(bounded) > 0 {
		r, size := utf8.DecodeLastRuneInString(bounded)
		if r == utf8.RuneError && size <= 1 {
			// A trailing partial rune; drop its bytes.
			bounded = bounded[:len(bounded)-1]
			continue
		}
		break
	}
	return bounded
}

// unsupportedDriver is installed when no driver is configured. It fails
// every launch with a clear error rather than silently doing nothing.
type unsupportedDriver struct{}

func (unsupportedDriver) Start(context.Context, Launch) (Process, error) {
	return nil, ErrDriverNotConfigured
}
