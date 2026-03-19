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

// dialWithLazyValidation dials an agent and, if the dial has not succeeded
// after delay, concurrently validates whether the workspace binding is stale.
// If validation discovers a different agent, the stale dial is canceled and
// the new agent is dialed instead.
//
// Outcomes:
//   - The dial succeeds before delay, so validation is skipped.
//   - The timer fires and validation confirms the same agent, so the original
//     dial continues.
//   - The timer fires and validation finds a different agent, so the stale
//     dial is canceled and the new agent is dialed.
//   - The dial fails before delay, so validation happens immediately and may
//     switch to the current agent.
func dialWithLazyValidation(
	ctx context.Context,
	agentID uuid.UUID,
	workspaceID uuid.UUID,
	dialFn DialFunc,
	validateFn ValidateFunc,
	delay time.Duration,
) (DialResult, error) {
	dialCtx, dialCancel := context.WithCancel(ctx)
	defer dialCancel()

	type dialOut struct {
		conn    workspacesdk.AgentConn
		release func()
		err     error
	}

	ch := make(chan dialOut, 1)
	go func() {
		conn, release, err := dialFn(dialCtx, agentID)
		ch <- dialOut{conn: conn, release: release, err: err}
	}()

	// waitForDial waits for the dial goroutine to resolve while still honoring
	// context cancellation. This avoids raw channel receives that could block
	// indefinitely if the parent context is canceled first.
	waitForDial := func(ctx context.Context) (dialOut, error) {
		select {
		case result := <-ch:
			return result, nil
		case <-ctx.Done():
			return dialOut{}, ctx.Err()
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case result := <-ch:
		if result.err == nil {
			return DialResult{
				Conn:    result.conn,
				Release: result.release,
				AgentID: agentID,
			}, nil
		}
		// Fast failure falls through to validate and maybe switch.

	case <-timer.C:
		currentID, err := validateFn(ctx, workspaceID)
		if err != nil || currentID == agentID {
			result, waitErr := waitForDial(ctx)
			if waitErr != nil {
				return DialResult{}, waitErr
			}
			if result.err != nil {
				return DialResult{}, result.err
			}
			return DialResult{
				Conn:    result.conn,
				Release: result.release,
				AgentID: agentID,
			}, nil
		}

		dialCancel()
		stale, waitErr := waitForDial(ctx)
		if waitErr != nil {
			return DialResult{}, waitErr
		}
		if stale.err == nil && stale.release != nil {
			stale.release()
		}

		conn, release, dialErr := dialFn(ctx, currentID)
		if dialErr != nil {
			return DialResult{}, dialErr
		}
		return DialResult{
			Conn:        conn,
			Release:     release,
			AgentID:     currentID,
			WasSwitched: true,
		}, nil

	case <-ctx.Done():
		return DialResult{}, ctx.Err()
	}

	currentID, err := validateFn(ctx, workspaceID)
	if err != nil {
		return DialResult{}, err
	}
	if currentID != agentID {
		conn, release, dialErr := dialFn(ctx, currentID)
		if dialErr != nil {
			return DialResult{}, dialErr
		}
		return DialResult{
			Conn:        conn,
			Release:     release,
			AgentID:     currentID,
			WasSwitched: true,
		}, nil
	}

	return DialResult{}, xerrors.New("agent is unreachable")
}
