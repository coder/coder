package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// DialResult contains the outcome of dialWithLazyValidation.
type DialResult struct {
	Conn        workspacesdk.AgentConn
	Release     func()
	AgentID     uuid.UUID // The agent that was actually dialed.
	WasSwitched bool      // True if validation discovered a different agent.
}

// DialFunc dials an agent by ID and returns a connection.
type DialFunc func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error)

// ValidateFunc returns the current agent ID for a workspace.
type ValidateFunc func(ctx context.Context, workspaceID uuid.UUID) (uuid.UUID, error)

type dialOut struct {
	conn    workspacesdk.AgentConn
	release func()
	err     error
}

// dialWithLazyValidation dials an agent and only consults the database if the
// original dial is slow or fails quickly. This keeps the common path free of
// latest-build lookups while still repairing stale bindings.
//
// Outcomes:
//   - The dial succeeds before delay, so validation is skipped.
//   - The timer fires and validation confirms the same agent, so the original
//     dial continues.
//   - The timer fires and validation finds a different agent, so the stale
//     dial is canceled and the new agent is dialed instead.
//   - The dial fails before delay, so validation runs immediately and either
//     switches to a different agent or retries the current one once.
func dialWithLazyValidation(
	ctx context.Context,
	agentID uuid.UUID,
	workspaceID uuid.UUID,
	dialFn DialFunc,
	validateFn ValidateFunc,
	delay time.Duration,
) (DialResult, error) {
	wrapErr := func(err error) error {
		return xerrors.Errorf("dial with lazy validation: %w", err)
	}

	dialCtx, dialCancel := context.WithCancel(ctx)
	results := make(chan dialOut, 1)
	go func() {
		conn, release, err := dialFn(dialCtx, agentID)
		results <- dialOut{conn: conn, release: release, err: err}
	}()

	drained := false
	defer func() {
		dialCancel()
		if drained {
			return
		}
		// Drain without blocking the caller. dialFn may take time to honor
		// cancellation, but any late-arriving successful connection still needs to
		// be released.
		go func() {
			result := <-results
			if result.err == nil && result.release != nil {
				result.release()
			}
		}()
	}()

	resultForAgent := func(dialedAgentID uuid.UUID, result dialOut, switched bool) DialResult {
		return DialResult{
			Conn:        result.conn,
			Release:     result.release,
			AgentID:     dialedAgentID,
			WasSwitched: switched,
		}
	}
	dialAgent := func(targetAgentID uuid.UUID, switched bool) (DialResult, error) {
		conn, release, err := dialFn(ctx, targetAgentID)
		if err != nil {
			return DialResult{}, wrapErr(err)
		}
		return resultForAgent(targetAgentID, dialOut{conn: conn, release: release}, switched), nil
	}
	preferReadyOriginalDial := func() (DialResult, bool) {
		select {
		case result := <-results:
			drained = true
			if result.err != nil {
				return DialResult{}, false
			}
			return resultForAgent(agentID, result, false), true
		default:
			return DialResult{}, false
		}
	}
	waitForOriginalDial := func(waitCtx context.Context) (DialResult, error) {
		select {
		case result := <-results:
			drained = true
			if result.err != nil {
				return DialResult{}, wrapErr(result.err)
			}
			return resultForAgent(agentID, result, false), nil
		case <-waitCtx.Done():
			if ready, ok := preferReadyOriginalDial(); ok {
				return ready, nil
			}
			return DialResult{}, waitCtx.Err()
		}
	}
	validateBinding := func() (uuid.UUID, error) {
		validatedAgentID, err := validateFn(ctx, workspaceID)
		if err != nil {
			if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
				return uuid.Nil, errChatHasNoWorkspaceAgent
			}
			return uuid.Nil, wrapErr(err)
		}
		return validatedAgentID, nil
	}
	resolveFastFailure := func() (DialResult, error) {
		validatedAgentID, err := validateBinding()
		if err != nil {
			return DialResult{}, err
		}
		if validatedAgentID == agentID {
			return dialAgent(agentID, false)
		}
		return dialAgent(validatedAgentID, true)
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case result := <-results:
		drained = true
		if result.err == nil {
			return resultForAgent(agentID, result, false), nil
		}
		return resolveFastFailure()

	case <-timer.C:
		validatedAgentID, validationErr := validateBinding()
		if validationErr != nil {
			if xerrors.Is(validationErr, errChatHasNoWorkspaceAgent) {
				dialCancel()
				return DialResult{}, validationErr
			}
			// Validation could not prove the binding was stale, so keep waiting on
			// the original dial.
			return waitForOriginalDial(ctx)
		}
		if validatedAgentID == agentID {
			// Validation confirmed the current binding, so keep waiting on the
			// original dial.
			return waitForOriginalDial(ctx)
		}
		// The original dial is stale. Cancel it first, then let the deferred drain
		// release any late result while we dial the validated agent immediately.
		dialCancel()
		return dialAgent(validatedAgentID, true)

	case <-ctx.Done():
		if ready, ok := preferReadyOriginalDial(); ok {
			return ready, nil
		}
		return DialResult{}, ctx.Err()
	}
}
