package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	defaultACPWaitTimeout = 5 * time.Minute

	// The value is arbitrary. We just want some delay between reconnect attempts
	// to avoid hammering the workspace agent.
	acpRetryDelay = 250 * time.Millisecond
)

type acpWaitPreview struct {
	callID  string
	publish func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
}

type acpWaitState struct {
	cursor     *workspacesdk.ACPCursor
	info       workspacesdk.ACPSession
	transcript *acpTranscript
	preview    acpWaitPreview
}

// update appends newline-delimited chat parts to the wait's result stream.
// A delta may contain multiple complete records.
func (p *acpWaitPreview) update(parts []acpTranscriptPart) error {
	if len(parts) == 0 {
		return nil
	}
	var payload strings.Builder
	encoder := json.NewEncoder(&payload)
	for _, part := range parts {
		part.Part.StripInternal()
		if err := encoder.Encode(part); err != nil {
			return xerrors.Errorf("encode ACP transcript part: %w", err)
		}
	}
	p.publish(codersdk.ChatMessageRoleTool, codersdk.ChatMessagePart{
		Type:        codersdk.ChatMessagePartTypeToolResult,
		ToolCallID:  p.callID,
		ToolName:    "acp_wait_agent",
		ResultDelta: payload.String(),
	})
	return nil
}

func (t acpToolset) wait(ctx context.Context, args acpWaitArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	timeout := defaultACPWaitTimeout
	if args.TimeoutSeconds != nil {
		timeout = time.Duration(*args.TimeoutSeconds) * time.Second
	}
	session, err := (acpSessionArgs{HarnessID: args.HarnessID, SessionID: args.SessionID, WorkingDirectory: args.WorkingDirectory}).sessionRef()
	if err != nil {
		return t.errorResponse(err, nil), nil
	}
	publish := chatloop.MessagePartPublisherFromContext(ctx)
	if publish == nil || call.ID == "" {
		return t.errorResponse(xerrors.New("ACP wait requires a message-part publisher and tool-call ID"), &session), nil
	}
	waitCtx, cancel := t.timeout(ctx, timeout, "acp-wait")
	defer cancel()
	state := acpWaitState{
		transcript: newACPTranscript(),
		preview:    acpWaitPreview{callID: call.ID, publish: publish},
	}
	var boundAgent uuid.UUID
	var waitErr error
	finished := false
	for waitCtx.Err() == nil {
		conn, err := t.resolve(waitCtx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
				waitErr = err
				break
			}
			if !t.waitBeforeRetry(waitCtx) {
				break
			}
			continue
		}
		agentID := conn.AgentID()
		// A workspace rebuild or restart can replace the agent while keeping the
		// workspace ID. Switching workspaces also changes the agent ID. The saved
		// transcript cursor cannot be reused against a different agent.
		if boundAgent != uuid.Nil && boundAgent != agentID {
			waitErr = xerrors.New("ACP workspace agent changed")
			break
		}
		boundAgent = agentID
		events, closer, watchErr := conn.WatchACPSession(waitCtx, t.logger, session, workspacesdk.ACPReadOptions{After: state.cursor})
		if watchErr != nil {
			var sdkErr *codersdk.Error
			if errors.As(watchErr, &sdkErr) {
				waitErr = watchErr
				break
			}
			if !t.waitBeforeRetry(waitCtx) {
				break
			}
			continue
		}
		finished, waitErr = consumeACPEvents(waitCtx, events, &state)
		if err := closer.Close(); err != nil {
			t.logger.Debug(ctx, "close ACP session watch", slog.Error(err), slog.F("session_id", session.SessionID))
		}
		if finished || waitErr != nil {
			break
		}
		if !t.waitBeforeRetry(waitCtx) {
			break
		}
	}
	if ctx.Err() != nil {
		return fantasy.ToolResponse{}, ctx.Err()
	}
	timedOut := !finished && waitCtx.Err() != nil && waitErr == nil
	if waitErr != nil {
		state.info.Status, state.info.Error = workspacesdk.ACPSessionStatusError, waitErr.Error()
	}
	return t.waitResponse(session, state.info, state.transcript, timedOut), nil
}

// consumeACPEvents returns true when the session finishes. A false result with
// no error lets the caller reconnect if its context is still active.
func consumeACPEvents(ctx context.Context, events <-chan workspacesdk.ACPEvent, state *acpWaitState) (done bool, err error) {
	// Each watch sends stored events, then a session-state snapshot, then
	// live events. Ignore terminal statuses before the snapshot because
	// they may belong to earlier turns.
	seenSnapshot := false
	for {
		select {
		case <-ctx.Done():
			return false, nil
		case event, ok := <-events:
			if !ok {
				return false, nil
			}
			if event.Kind == workspacesdk.ACPEventKindSnapshot {
				if event.Session == nil {
					return false, xerrors.New("ACP snapshot has no session")
				}
				state.info = *event.Session
				state.cursor = &event.Cursor
				seenSnapshot = true
				if state.info.Status == workspacesdk.ACPSessionStatusIdle || state.info.Status == workspacesdk.ACPSessionStatusError {
					return true, nil
				}
				continue
			}
			if state.cursor != nil && event.Cursor.Seq <= state.cursor.Seq {
				continue
			}
			if state.cursor != nil && event.Cursor.Seq != state.cursor.Seq+1 {
				return false, nil
			}
			state.cursor = &event.Cursor
			parts, err := state.transcript.consume(event)
			if err != nil {
				return false, err
			}
			if err := state.preview.update(parts); err != nil {
				return false, err
			}
			if event.Session != nil && seenSnapshot {
				state.info = *event.Session
				if event.Kind == workspacesdk.ACPEventKindStatus && (state.info.Status == workspacesdk.ACPSessionStatusIdle || state.info.Status == workspacesdk.ACPSessionStatusError) {
					return true, nil
				}
			}
		}
	}
}

func (t acpToolset) waitBeforeRetry(ctx context.Context) bool {
	timer := t.clock.NewTimer(acpRetryDelay, "acp-retry")
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (t acpToolset) waitResponse(session workspacesdk.ACPSessionID, info workspacesdk.ACPSession, transcript *acpTranscript, timedOut bool) fantasy.ToolResponse {
	sessionInfo := t.sessionResult(session)
	if info.HarnessDisplayName != "" {
		sessionInfo.HarnessDisplayName = new(info.HarnessDisplayName)
	}
	model := map[string]any{
		"session_id": sessionInfo.SessionID,
		"harness_id": sessionInfo.HarnessID,
		"status":     info.Status,
		"timed_out":  timedOut,
		"response":   transcript.response.String(),
	}
	user := map[string]any{
		"session_id":        sessionInfo.SessionID,
		"harness_id":        sessionInfo.HarnessID,
		"working_directory": sessionInfo.WorkingDirectory,
		"status":            info.Status,
		"timed_out":         timedOut,
		"messages":          transcript.messages,
	}
	if sessionInfo.HarnessDisplayName != nil {
		model["harness_display_name"] = *sessionInfo.HarnessDisplayName
		user["harness_display_name"] = *sessionInfo.HarnessDisplayName
	}
	if info.Error != "" {
		model["error"], user["error"] = info.Error, info.Error
	}
	response := acpResponse(model)
	response.IsError = info.Status == workspacesdk.ACPSessionStatusError
	return chattool.WithUserResult(response, user)
}
