package chatd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// workspaceFileDeliveryLease is how long a claimed file stays leased
	// to this replica, plus the time its upload takes at
	// workspaceFileDeliveryMinUploadBytesPerSecond.
	workspaceFileDeliveryLease = 5 * time.Minute
	// workspaceFileDeliveryMinUploadBytesPerSecond is the slowest upload
	// rate a lease leaves time for.
	workspaceFileDeliveryMinUploadBytesPerSecond = 128 << 10
	// workspaceFileDeliveryLeaseMargin ends an attempt early enough to
	// record its outcome before the lease expires, so no other replica
	// works on the file at the same time.
	workspaceFileDeliveryLeaseMargin = time.Minute
	workspaceFileDeliveryRetryDelay  = time.Minute
)

// workspaceFileDeliveryFailure is a failed delivery attempt and how the
// attempt counts: one of the database.ChatWorkspaceFileDeliveryFailure
// modes.
type workspaceFileDeliveryFailure struct {
	mode string
	err  error
}

func refundWorkspaceFileDelivery(err error) *workspaceFileDeliveryFailure {
	return &workspaceFileDeliveryFailure{mode: database.ChatWorkspaceFileDeliveryFailureRefund, err: err}
}

func retryWorkspaceFileDelivery(err error) *workspaceFileDeliveryFailure {
	return &workspaceFileDeliveryFailure{mode: database.ChatWorkspaceFileDeliveryFailureRetry, err: err}
}

func parkWorkspaceFileDelivery(err error) *workspaceFileDeliveryFailure {
	return &workspaceFileDeliveryFailure{mode: database.ChatWorkspaceFileDeliveryFailurePark, err: err}
}

// linksWorkspaceAttachments reports whether parts reference files that the
// delivery loop copies into the chat's workspace.
func linksWorkspaceAttachments(parts []codersdk.ChatMessagePart) bool {
	return slices.ContainsFunc(parts, func(part codersdk.ChatMessagePart) bool {
		return part.Type == codersdk.ChatMessagePartTypeWorkspaceAttachment && part.FileID.Valid
	})
}

// wakeWorkspaceFileDelivery requests a delivery sweep on this replica.
func (p *Server) wakeWorkspaceFileDelivery() {
	if p.chatWorker != nil {
		wake(p.chatWorker.fileDeliveryWakeCh)
	}
}

// workspaceFileDeliveryLoop copies workspace chat files into their chats'
// workspaces. Postgres holds the queue; agent startup events and
// in-process pokes only wake the loop, and a periodic sweep covers lost
// wake-ups, retries and expired leases. The loop never writes chat
// messages, because a message write cancels a running turn.
func (w *chatWorker) workspaceFileDeliveryLoop(ctx context.Context) {
	unsubscribe, err := w.opts.Pubsub.SubscribeWithErr(
		wspubsub.AgentStartupFinishedChannel,
		wspubsub.HandleAgentStartupFinished(func(ctx context.Context, event wspubsub.AgentStartupFinishedEvent, err error) {
			if err != nil {
				w.opts.Logger.Warn(ctx, "agent startup event error, sweeping workspace files", slogError(err))
			} else {
				w.opts.Logger.Debug(ctx, "agent startup finished, sweeping workspace files",
					slog.F("workspace_id", event.WorkspaceID),
					slog.F("agent_id", event.AgentID),
				)
			}
			wake(w.fileDeliveryWakeCh)
		}),
	)
	if err != nil {
		w.opts.Logger.Error(ctx, "subscribe to agent startup events, workspace files are delivered by sweeps only", slogError(err))
	} else {
		defer unsubscribe()
	}

	// Catch up on files that became deliverable while this replica was
	// not subscribed.
	w.deliverWorkspaceFiles(ctx)

	ticker := w.opts.Clock.NewTicker(w.opts.WorkspaceFileDeliveryInterval, "chatworker", "workspace-file-delivery")
	defer ticker.Stop("chatworker", "workspace-file-delivery")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.fileDeliveryWakeCh:
		}
		w.deliverWorkspaceFiles(ctx)
	}
}

// deliverWorkspaceFiles delivers due files until none is left. It claims
// one file at a time so a lease only covers the delivery in progress and
// the replica holds one file's content at a time.
func (w *chatWorker) deliverWorkspaceFiles(ctx context.Context) {
	for ctx.Err() == nil {
		now := w.opts.Clock.Now("chatworker", "workspace-file-delivery")
		rows, err := w.opts.Store.AcquireChatWorkspaceFileDeliveries(ctx, database.AcquireChatWorkspaceFileDeliveriesParams{
			Now:                     now,
			LeaseExpiresAt:          now.Add(workspaceFileDeliveryLease),
			MinUploadBytesPerSecond: workspaceFileDeliveryMinUploadBytesPerSecond,
			ChatAgentSuffix:         agentselect.Suffix,
			MaxAttempts:             database.ChatWorkspaceFileMaxDeliveryAttempts,
			ChatID:                  uuid.NullUUID{},
			LimitVal:                1,
		})
		if err != nil {
			if ctx.Err() == nil {
				w.opts.Logger.Warn(ctx, "claim workspace chat files for delivery", slogError(err))
			}
			return
		}
		if len(rows) == 0 {
			return
		}
		w.deliverWorkspaceFile(ctx, rows[0])
	}
}

func (w *chatWorker) deliverWorkspaceFile(ctx context.Context, row database.AcquireChatWorkspaceFileDeliveriesRow) {
	logger := w.opts.Logger.With(
		slog.F("chat_id", row.ChatID),
		slog.F("file_id", row.ID),
		slog.F("workspace_id", row.WorkspaceID.UUID),
		slog.F("attempt", row.WorkspaceDeliveryAttempts),
	)
	claimedAt := w.opts.Clock.Now("chatworker", "workspace-file-delivery")
	attemptCtx, cancel := context.WithTimeout(ctx, row.LeaseExpiresAt.Sub(claimedAt)-workspaceFileDeliveryLeaseMargin)
	resp, failure := w.copyWorkspaceFile(attemptCtx, row)
	cancel()
	if ctx.Err() != nil {
		fields := []slog.Field{slogError(ctx.Err())}
		if failure != nil {
			fields = append(fields, slog.F("attempt_error", failure.err.Error()))
		}
		logger.Info(ctx, "workspace chat file delivery interrupted, a later sweep retries it once the lease expires", fields...)
		return
	}

	now := w.opts.Clock.Now("chatworker", "workspace-file-delivery")
	if failure == nil {
		// Record the home-relative path the prompt names, not the
		// absolute path, which depends on the agent's home directory.
		deliveredPath := chatfiles.WorkspaceUploadFileHomePath(row.ChatID.String(), row.ID.String(), resp.Name)
		completed, err := w.opts.Store.CompleteChatWorkspaceFileDelivery(ctx, database.CompleteChatWorkspaceFileDeliveryParams{
			DeliveredAt:          now,
			DeliveredPath:        deliveredPath,
			DeliveredWorkspaceID: row.WorkspaceID.UUID,
			ID:                   row.ID,
			LeaseExpiresAt:       row.LeaseExpiresAt,
		})
		if err != nil {
			// The agent upload is write-once by file ID, so the
			// redelivery after the lease expires acknowledges this copy.
			logger.Error(ctx, "record workspace chat file delivery", slogError(err))
			return
		}
		if completed == 0 {
			logger.Info(ctx, "workspace chat file delivery outcome dropped, the file is no longer under this claim", slog.F("path", resp.Path))
			return
		}
		logger.Info(ctx, "delivered workspace chat file", slog.F("path", resp.Path))
		return
	}

	logger.Warn(ctx, "workspace chat file delivery failed", slog.F("mode", failure.mode), slogError(failure.err))
	failed, err := w.opts.Store.FailChatWorkspaceFileDelivery(ctx, database.FailChatWorkspaceFileDeliveryParams{
		Error:          failure.err.Error(),
		NextAttemptAt:  now.Add(workspaceFileDeliveryRetryDelay),
		Mode:           failure.mode,
		MaxAttempts:    database.ChatWorkspaceFileMaxDeliveryAttempts,
		ID:             row.ID,
		LeaseExpiresAt: row.LeaseExpiresAt,
	})
	if err != nil {
		logger.Error(ctx, "record failed workspace chat file delivery", slogError(err))
		return
	}
	if failed == 0 {
		logger.Info(ctx, "workspace chat file delivery outcome dropped, the file is no longer under this claim")
	}
}

// copyWorkspaceFile writes the file into the workspace through the agent
// that serves the chat.
func (w *chatWorker) copyWorkspaceFile(ctx context.Context, row database.AcquireChatWorkspaceFileDeliveriesRow) (workspacesdk.AgentUploadChatFileResponse, *workspaceFileDeliveryFailure) {
	chat, err := w.opts.Store.GetChatByID(ctx, row.ChatID)
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, refundWorkspaceFileDelivery(xerrors.Errorf("load chat: %w", err))
	}
	agents, err := w.opts.Store.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, row.WorkspaceID.UUID)
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, refundWorkspaceFileDelivery(xerrors.Errorf("load workspace agents: %w", err))
	}
	agent, err := agentselect.ForChat(chat, agents)
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, parkWorkspaceFileDelivery(xerrors.Errorf("select the workspace agent: %w", err))
	}
	now := w.opts.Clock.Now("chatworker", "workspace-file-delivery")
	if status := agent.Status(now, w.server.agentInactiveDisconnectTimeout).Status; status != database.WorkspaceAgentStatusConnected {
		return workspacesdk.AgentUploadChatFileResponse{}, refundWorkspaceFileDelivery(xerrors.Errorf("workspace agent %q is %s, not connected", agent.Name, status))
	}
	file, err := w.opts.Store.GetChatFileByID(ctx, row.ID)
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, refundWorkspaceFileDelivery(xerrors.Errorf("load file content: %w", err))
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, w.server.dialTimeout)
	defer dialCancel()
	conn, release, err := w.server.agentConnFn(dialCtx, agent.ID)
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, refundWorkspaceFileDelivery(xerrors.Errorf("dial workspace agent %q: %w", agent.Name, err))
	}
	defer release()
	conn.SetExtraHeaders(http.Header{workspacesdk.CoderChatIDHeader: {chat.ID.String()}})

	resp, err := conn.UploadChatFile(ctx, workspacesdk.UploadChatFileRequest{
		ChatID: chat.ID.String(),
		Name:   file.Name,
		FileID: file.ID,
		Body:   bytes.NewReader(file.Data),
	})
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, classifyWorkspaceFileUploadError(err)
	}
	// A write-once upload reports a file that already exists at the target
	// instead of replacing it, so a size mismatch means the workspace holds
	// a different file and the stored bytes must survive.
	if resp.Size != int64(len(file.Data)) {
		return workspacesdk.AgentUploadChatFileResponse{}, parkWorkspaceFileDelivery(xerrors.Errorf(
			"the workspace already has a different file at %s (%d bytes, expected %d bytes); remove that file from the workspace, then retry",
			resp.Path, resp.Size, len(file.Data),
		))
	}
	return resp, nil
}

// classifyWorkspaceFileUploadError parks a file the agent rejected, since
// repeating the same request cannot succeed, and retries server and
// transport errors.
func classifyWorkspaceFileUploadError(err error) *workspaceFileDeliveryFailure {
	var sdkErr *codersdk.Error
	if !errors.As(err, &sdkErr) {
		return retryWorkspaceFileDelivery(xerrors.Errorf("upload to the workspace agent: %w", err))
	}
	status := sdkErr.StatusCode()
	if status < http.StatusBadRequest || status >= http.StatusInternalServerError {
		return retryWorkspaceFileDelivery(xerrors.Errorf("upload to the workspace agent: %w", err))
	}
	reason := sdkErr.Message
	if sdkErr.Detail != "" {
		reason = fmt.Sprintf("%s: %s", reason, sdkErr.Detail)
	}
	// Agents that predate write-once uploads answer 400 to the file_id
	// parameter, and agents that predate chat uploads answer 404.
	if status == http.StatusBadRequest || status == http.StatusNotFound {
		return parkWorkspaceFileDelivery(xerrors.Errorf("the workspace agent rejected the upload with status %d (%s); update the workspace agent, for example by restarting the workspace, then retry", status, reason))
	}
	return parkWorkspaceFileDelivery(xerrors.Errorf("the workspace agent rejected the upload with status %d (%s)", status, reason))
}
