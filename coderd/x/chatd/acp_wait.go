package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type acpWaitPreview struct {
	callID  string
	publish func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
	started bool
	last    string
}

func (p *acpWaitPreview) part(part codersdk.ChatMessagePart) {
	part.Type, part.ToolCallID, part.ToolName = codersdk.ChatMessagePartTypeToolResult, p.callID, "acp_wait_agent"
	p.publish(codersdk.ChatMessageRoleTool, part)
}

func (p *acpWaitPreview) update(messages []codersdk.ChatACPTranscriptMessage) {
	encoded, _ := json.Marshal(struct {
		Messages []codersdk.ChatACPTranscriptMessage `json:"messages"`
	}{Messages: messages})
	payload := string(encoded)
	if p.started && payload == p.last {
		return
	}
	// Tool updates replace existing parts, so publish the reduced transcript
	// atomically rather than exposing intermediate or duplicate tool states.
	p.part(codersdk.ChatMessagePart{ResultReset: p.started, ResultDelta: payload})
	p.started, p.last = true, payload
}

func (o acpToolOptions) wait(ctx context.Context, args acpWaitArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	seconds := 300
	if args.TimeoutSeconds != nil {
		seconds = *args.TimeoutSeconds
	}
	if seconds < 1 || seconds > 300 {
		return acpError(xerrors.New("timeout_seconds must be 1 through 300"), nil), nil
	}
	session, err := o.session(ctx, args.SessionID)
	if err != nil {
		return acpError(err, nil), nil
	}
	publish := chatloop.MessagePartPublisherFromContext(ctx)
	if publish == nil || call.ID == "" {
		return acpError(xerrors.New("ACP wait requires a message-part publisher and tool-call ID"), &session), nil
	}
	waitCtx, cancel := o.timeout(ctx, time.Duration(seconds)*time.Second, "acp-wait")
	defer cancel()
	preview := acpWaitPreview{callID: call.ID, publish: publish}
	var transcript acpTranscript
	transcript.reset()
	var cursor *workspacesdk.ACPCursor
	var boundAgent uuid.UUID
	info := workspacesdk.ACPSession{Status: workspacesdk.ACPSessionStatusStarting}
	var waitErr error
	finished := false
	for !finished && waitCtx.Err() == nil {
		var conn workspacesdk.AgentConn
		var agentID uuid.UUID
		conn, agentID, err = o.connection(waitCtx, session)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errACPWorkspaceChanged) || dbauthz.IsNotAuthorizedError(err) {
				waitErr = err
				break
			}
			if !o.waitReconnect(waitCtx) {
				break
			}
			continue
		}
		if boundAgent != uuid.Nil && boundAgent != agentID {
			cursor = nil
			transcript.reset()
			info = workspacesdk.ACPSession{Status: workspacesdk.ACPSessionStatusStarting}
			if preview.started {
				preview.update(transcript.messages)
			}
		}
		boundAgent = agentID
		events, closer, watchErr := conn.WatchACPSession(waitCtx, o.logger, acpNativeID(session), workspacesdk.ACPReadOptions{After: cursor})
		if watchErr != nil {
			var sdkErr *codersdk.Error
			if errors.As(watchErr, &sdkErr) {
				if sdkErr.StatusCode() == http.StatusConflict && cursor != nil {
					cursor = nil
					transcript.reset()
					if preview.started {
						preview.update(transcript.messages)
					}
					continue
				}
				waitErr = watchErr
				break
			}
			if !o.waitReconnect(waitCtx) {
				break
			}
			continue
		}
		baseline := true
		connected := true
		for connected && !finished {
			select {
			case <-waitCtx.Done():
				connected = false
			case event, ok := <-events:
				if !ok {
					connected = false
					break
				}
				if event.Kind == workspacesdk.ACPEventKindSnapshot {
					if event.Session == nil {
						waitErr = xerrors.New("ACP snapshot has no session")
						finished = true
						break
					}
					if cursor != nil && event.Cursor.Epoch != cursor.Epoch {
						transcript.reset()
					}
					info = *event.Session
					cursor = &event.Cursor
					baseline = false
					// Rebuild once after baseline replay so neither old turns
					// nor intermediate historical statuses reach the preview.
					preview.update(transcript.messages)
					finished = info.Status == workspacesdk.ACPSessionStatusIdle || info.Status == workspacesdk.ACPSessionStatusError
					continue
				}
				if cursor != nil && event.Cursor.Epoch == cursor.Epoch && event.Cursor.Seq <= cursor.Seq {
					continue
				}
				if cursor != nil && event.Cursor.Epoch == cursor.Epoch && event.Cursor.Seq != cursor.Seq+1 {
					connected = false
					break
				}
				if cursor != nil && event.Cursor.Epoch != cursor.Epoch {
					transcript.reset()
					if !baseline {
						preview.update(transcript.messages)
					}
				}
				cursor = &event.Cursor
				changed, _, consumeErr := transcript.consume(event)
				if consumeErr != nil {
					waitErr = consumeErr
					finished = true
					break
				}
				if !baseline && changed {
					preview.update(transcript.messages)
				}
				if event.Session != nil && !baseline {
					info = *event.Session
					if event.Kind == workspacesdk.ACPEventKindStatus {
						finished = info.Status == workspacesdk.ACPSessionStatusIdle || info.Status == workspacesdk.ACPSessionStatusError
					}
				}
			}
		}
		_ = closer.Close()
		if !finished && !o.waitReconnect(waitCtx) {
			break
		}
	}
	if ctx.Err() != nil {
		return fantasy.ToolResponse{}, ctx.Err()
	}
	timedOut := !finished && waitCtx.Err() != nil && waitErr == nil
	if waitErr != nil {
		info.Status, info.Error = workspacesdk.ACPSessionStatusError, waitErr.Error()
	}
	return acpWaitResponse(session, info, &transcript, timedOut), nil
}

func (o acpToolOptions) waitReconnect(ctx context.Context) bool {
	timer := o.clock.NewTimer(250*time.Millisecond, "acp-reconnect")
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func acpWaitResponse(session database.AgentsAcpSession, info workspacesdk.ACPSession, transcript *acpTranscript, timedOut bool) fantasy.ToolResponse {
	model := map[string]any{"session_id": session.ID.String(), "harness": session.HarnessSlug, "harness_display_name": session.HarnessDisplayName, "status": info.Status, "timed_out": timedOut, "response": transcript.response.String()}
	user := map[string]any{"session_id": session.ID.String(), "harness": session.HarnessSlug, "harness_display_name": session.HarnessDisplayName, "status": info.Status, "timed_out": timedOut, "messages": transcript.messages, "history_complete": info.HistoryComplete}
	if !info.HistoryComplete {
		model["history_complete"] = false
	}
	if info.Error != "" {
		model["error"], user["error"] = info.Error, info.Error
	}
	response := acpResponse(model)
	response.IsError = info.Status == workspacesdk.ACPSessionStatusError
	return chattool.WithUserResult(response, user)
}
