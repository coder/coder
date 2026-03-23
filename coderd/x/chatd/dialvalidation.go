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

// dialAttempt owns one in-flight dial goroutine and ensures any late-arriving
// successful connection is released if the caller stops caring about the
// result.
type dialAttempt struct {
	cancel      context.CancelFunc
	results     chan dialOut
	resultTaken bool
}

func startDialAttempt(ctx context.Context, agentID uuid.UUID, dialFn DialFunc) *dialAttempt {
	dialCtx, cancel := context.WithCancel(ctx)
	results := make(chan dialOut, 1)
	go func() {
		conn, release, err := dialFn(dialCtx, agentID)
		results <- dialOut{conn: conn, release: release, err: err}
	}()
	return &dialAttempt{cancel: cancel, results: results}
}

func (a *dialAttempt) await(ctx context.Context) (dialOut, error) {
	select {
	case result := <-a.results:
		a.resultTaken = true
		return result, nil
	case <-ctx.Done():
		return dialOut{}, ctx.Err()
	}
}

func (a *dialAttempt) takeIfReady() (dialOut, bool) {
	select {
	case result := <-a.results:
		a.resultTaken = true
		return result, true
	default:
		return dialOut{}, false
	}
}

func (a *dialAttempt) take(result dialOut) dialOut {
	a.resultTaken = true
	return result
}

func (a *dialAttempt) cancelAndAwait() dialOut {
	a.cancel()
	result := <-a.results
	a.resultTaken = true
	return result
}

func (a *dialAttempt) cleanup() {
	a.cancel()
	if a.resultTaken {
		return
	}
	// Launch a small goroutine to drain without blocking the caller. dialFn may
	// take time to honor cancellation.
	go func() {
		result := <-a.results
		if result.err == nil && result.release != nil {
			result.release()
		}
	}()
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
//     switches to a different agent or reports that the current one is
//     unreachable.
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
	originalDial := startDialAttempt(ctx, agentID, dialFn)
	defer originalDial.cleanup()

	resultForAgent := func(dialedAgentID uuid.UUID, result dialOut, switched bool) DialResult {
		return DialResult{
			Conn:        result.conn,
			Release:     result.release,
			AgentID:     dialedAgentID,
			WasSwitched: switched,
		}
	}
	preferReadyOriginalDial := func() (DialResult, bool) {
		result, ok := originalDial.takeIfReady()
		if !ok || result.err != nil {
			return DialResult{}, false
		}
		return resultForAgent(agentID, result, false), true
	}
	waitForOriginalDial := func(waitCtx context.Context) (DialResult, error) {
		result, err := originalDial.await(waitCtx)
		if err != nil {
			if waitCtx.Err() != nil {
				if ready, ok := preferReadyOriginalDial(); ok {
					return ready, nil
				}
			}
			return DialResult{}, err
		}
		if result.err != nil {
			return DialResult{}, wrapErr(result.err)
		}
		return resultForAgent(agentID, result, false), nil
	}
	dialValidatedAgent := func(validatedAgentID uuid.UUID) (DialResult, error) {
		conn, release, err := dialFn(ctx, validatedAgentID)
		if err != nil {
			return DialResult{}, wrapErr(err)
		}
		return DialResult{
			Conn:        conn,
			Release:     release,
			AgentID:     validatedAgentID,
			WasSwitched: true,
		}, nil
	}
	validateBinding := func() (uuid.UUID, error) {
		validatedAgentID, err := validateFn(ctx, workspaceID)
		if err != nil {
			return uuid.Nil, wrapErr(err)
		}
		return validatedAgentID, nil
	}
	handleFastFailure := func() (DialResult, error) {
		validatedAgentID, err := validateBinding()
		if err != nil {
			return DialResult{}, err
		}
		// Phase 2 only runs after a fast failure from the original dial. When
		// validation still points at the same agent, the binding is current and
		// the agent is genuinely unreachable.
		if validatedAgentID == agentID {
			return DialResult{}, xerrors.New("agent is unreachable")
		}
		return dialValidatedAgent(validatedAgentID)
	}
	switchSlowDial := func(validatedAgentID uuid.UUID) (DialResult, error) {
		staleResult := originalDial.cancelAndAwait()
		if staleResult.err == nil && staleResult.release != nil {
			staleResult.release()
		}
		return dialValidatedAgent(validatedAgentID)
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case result := <-originalDial.results:
		result = originalDial.take(result)
		if result.err == nil {
			return resultForAgent(agentID, result, false), nil
		}
		return handleFastFailure()

	case <-timer.C:
		validatedAgentID, validationErr := validateFn(ctx, workspaceID)
		if validationErr != nil || validatedAgentID == agentID {
			// Validation could not prove the binding was stale, so keep waiting on
			// the original dial.
			return waitForOriginalDial(ctx)
		}
		return switchSlowDial(validatedAgentID)

	case <-ctx.Done():
		if ready, ok := preferReadyOriginalDial(); ok {
			return ready, nil
		}
		return DialResult{}, ctx.Err()
	}
}
