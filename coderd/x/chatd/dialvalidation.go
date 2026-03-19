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
// The goroutine result channel is consumed exactly once on every path. When
// the main flow does not consume it, a deferred cleanup goroutine drains the
// channel and releases any late-arriving connection.
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

	// drained tracks whether ch has been consumed by the main flow.
	// When the function returns without consuming ch, the deferred
	// cleanup cancels dialCtx and releases any late-arriving conn.
	var drained bool
	defer func() {
		dialCancel()
		if drained {
			return
		}
		// The goroutine will terminate because dialCtx is canceled.
		// Launch a small goroutine to drain without blocking the
		// caller (dialFn may take time to honor cancellation).
		go func() {
			r := <-ch
			if r.err == nil && r.release != nil {
				r.release()
			}
		}()
	}()

	timer := time.NewTimer(delay)
	defer timer.Stop()

	// Phase 1: race dial completion against the validation delay.
	select {
	case r := <-ch:
		drained = true
		if r.err == nil {
			return DialResult{
				Conn: r.conn, Release: r.release, AgentID: agentID,
			}, nil
		}
		// Fast failure — fall through to Phase 2.

	case <-timer.C:
		// Dial still in progress. Validate the binding.
		currentID, vErr := validateFn(ctx, workspaceID)
		if vErr != nil || currentID == agentID {
			// Same agent or validation error: let the original
			// dial finish.
			select {
			case r := <-ch:
				drained = true
				if r.err != nil {
					return DialResult{}, xerrors.Errorf(
						"dial with lazy validation: %w", r.err)
				}
				return DialResult{
					Conn: r.conn, Release: r.release, AgentID: agentID,
				}, nil
			case <-ctx.Done():
				// Prefer a ready result over cancellation.
				select {
				case r := <-ch:
					drained = true
					if r.err == nil {
						return DialResult{
							Conn: r.conn, Release: r.release,
							AgentID: agentID,
						}, nil
					}
				default:
				}
				// Defer will clean up.
				return DialResult{}, ctx.Err()
			}
		}

		// Different agent: cancel stale dial, drain, switch.
		dialCancel()
		stale := <-ch // safe: dialCtx canceled, goroutine will finish
		drained = true
		if stale.err == nil && stale.release != nil {
			stale.release()
		}

		conn, release, dialErr := dialFn(ctx, currentID)
		if dialErr != nil {
			return DialResult{}, xerrors.Errorf(
				"dial with lazy validation: %w", dialErr)
		}
		return DialResult{
			Conn: conn, Release: release,
			AgentID: currentID, WasSwitched: true,
		}, nil

	case <-ctx.Done():
		// Prefer a ready result over cancellation.
		select {
		case r := <-ch:
			drained = true
			if r.err == nil {
				return DialResult{
					Conn: r.conn, Release: r.release, AgentID: agentID,
				}, nil
			}
		default:
		}
		// Defer will clean up.
		return DialResult{}, ctx.Err()
	}

	// Phase 2: reached only on fast dial failure (ch already drained).
	// Validate and maybe switch to the current agent.
	currentID, err := validateFn(ctx, workspaceID)
	if err != nil {
		return DialResult{}, xerrors.Errorf(
			"dial with lazy validation: %w", err)
	}
	if currentID == agentID {
		return DialResult{}, xerrors.New("agent is unreachable")
	}
	conn, release, dialErr := dialFn(ctx, currentID)
	if dialErr != nil {
		return DialResult{}, xerrors.Errorf(
			"dial with lazy validation: %w", dialErr)
	}
	return DialResult{
		Conn: conn, Release: release,
		AgentID: currentID, WasSwitched: true,
	}, nil
}
