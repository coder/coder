package chatd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
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
	// workspaceFileDeliveryMaxDisconnectedDelay caps the retry delay of a
	// file whose agent is not connected.
	workspaceFileDeliveryMaxDisconnectedDelay = 10 * time.Minute
	// workspaceFileDeliveryUsageInterval bounds how often a delivery
	// records workspace usage while its content streams, the cadence of
	// the direct upload path and the chat heartbeat.
	workspaceFileDeliveryUsageInterval = 30 * time.Second
	// workspaceFileDeliveryStallTimeout ends an upload attempt when the
	// agent takes no body bytes for this long, or does not answer for this
	// long after the last one, so a stalled agent holds the replica's
	// serial delivery loop for at most this long instead of the lease.
	workspaceFileDeliveryStallTimeout = time.Minute
)

var errWorkspaceFileDeliveryStalled = xerrors.Errorf("the workspace agent accepted no data for %s", workspaceFileDeliveryStallTimeout)

// workspaceFileDeliveryFailure is a failed delivery attempt and how the
// attempt counts: one of the database.ChatWorkspaceFileDeliveryFailure
// modes.
type workspaceFileDeliveryFailure struct {
	mode string
	// err is stored on the file, where the model and the UI read it.
	err error
	// cause is the full error behind err for the log, when err leaves
	// out details such as the agent's tailnet request URL.
	cause error
	// retryAfter replaces workspaceFileDeliveryRetryDelay when set.
	retryAfter time.Duration
	// quiet logs the failure at debug level.
	quiet bool
}

func (f *workspaceFileDeliveryFailure) logError() error {
	if f.cause != nil {
		return f.cause
	}
	return f.err
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

// RequestWorkspaceFileDelivery requests a delivery sweep on this replica, for
// a change outside chatd that can make workspace files deliverable, such
// as binding a chat to a running workspace.
func (p *Server) RequestWorkspaceFileDelivery() {
	p.wakeWorkspaceFileDelivery()
}

// retryAttachedWorkspaceFiles gives the workspace files that parts attach
// a fresh delivery attempt budget in the message's transaction. Removing an
// attachment before delivery parks its file, and the claim skips a parked
// file until the agent starts again, so a message that attaches the same
// file again must reset it.
func (p *Server) retryAttachedWorkspaceFiles(ctx context.Context, store database.Store, chatID uuid.UUID, parts []codersdk.ChatMessagePart) error {
	var fileIDs []uuid.UUID
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeWorkspaceAttachment && part.FileID.Valid {
			fileIDs = append(fileIDs, part.FileID.UUID)
		}
	}
	if len(fileIDs) == 0 {
		return nil
	}
	if _, err := store.RetryChatWorkspaceFileDeliveriesByIDs(ctx, database.RetryChatWorkspaceFileDeliveriesByIDsParams{
		ChatID:  chatID,
		FileIds: fileIDs,
		Now:     p.clock.Now(),
	}); err != nil {
		return xerrors.Errorf("retry attached workspace files: %w", err)
	}
	return nil
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
// wake-ups, retries and expired leases. Every agent startup also gives
// the failed files of the workspace's chats a fresh attempt budget, which
// the claim query derives from the agent's ready_at, because a restart is
// how a user updates an agent that rejected them. The loop never writes
// chat messages, because a message write cancels a running turn.
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
	nextAttemptAt := now.Add(workspaceFileDeliveryRetryDelay)
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
		if completed > 0 {
			logger.Info(ctx, "delivered workspace chat file", slog.F("path", resp.Path))
			return
		}
		// The file left this claim, for example because the per-chat
		// file cap evicted it or another replica claimed it after the
		// lease expired, which fences the refund below too. Otherwise no
		// chat of the file is bound to the workspace anymore, and the
		// file is due for the chat's new workspace now.
		failure = refundWorkspaceFileDelivery(xerrors.Errorf("the copy at %s was not recorded because no chat of the file is bound to workspace %s anymore", resp.Path, row.WorkspaceID.UUID))
		nextAttemptAt = now
	}

	if failure.retryAfter > 0 {
		nextAttemptAt = now.Add(failure.retryAfter)
	}
	failed, err := w.opts.Store.FailChatWorkspaceFileDelivery(ctx, database.FailChatWorkspaceFileDeliveryParams{
		Error:          failure.err.Error(),
		NextAttemptAt:  nextAttemptAt,
		Mode:           failure.mode,
		MaxAttempts:    database.ChatWorkspaceFileMaxDeliveryAttempts,
		ID:             row.ID,
		LeaseExpiresAt: row.LeaseExpiresAt,
	})
	if err != nil {
		logger.Error(ctx, "record failed workspace chat file delivery", slog.F("mode", failure.mode), slog.F("attempt_error", failure.logError().Error()), slogError(err))
		return
	}
	if failed == 0 {
		logger.Info(ctx, "workspace chat file delivery outcome dropped, the file is no longer under this claim", slog.F("attempt_error", failure.logError().Error()))
		return
	}
	fields := []slog.Field{slog.F("mode", failure.mode), slog.F("next_attempt_at", nextAttemptAt), slogError(failure.logError())}
	if failure.quiet {
		logger.Debug(ctx, "workspace chat file delivery failed", fields...)
		return
	}
	logger.Warn(ctx, "workspace chat file delivery failed", fields...)
}

// copyWorkspaceFile writes the file into the workspace through the agent
// that serves the chat.
func (w *chatWorker) copyWorkspaceFile(ctx context.Context, row database.AcquireChatWorkspaceFileDeliveriesRow) (workspacesdk.AgentUploadChatFileResponse, *workspaceFileDeliveryFailure) {
	chat, err := w.opts.Store.GetChatByID(ctx, row.ChatID)
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, refundWorkspaceFileDelivery(xerrors.Errorf("load chat: %w", err))
	}
	attached, err := w.opts.Store.IsChatWorkspaceFileAttached(ctx, database.IsChatWorkspaceFileAttachedParams{ChatID: chat.ID, FileID: row.ID})
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, refundWorkspaceFileDelivery(xerrors.Errorf("check the chat's messages for the file: %w", err))
	}
	if !attached {
		// The user removed the file before it was copied, so it waits for
		// an explicit retry instead.
		failure := parkWorkspaceFileDelivery(xerrors.New("no message in the chat attaches the file anymore"))
		failure.quiet = true
		return workspacesdk.AgentUploadChatFileResponse{}, failure
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
		failure := refundWorkspaceFileDelivery(xerrors.Errorf("workspace agent %q is %s, not connected", agent.Name, status))
		// An agent that goes away without a lifecycle change still looks
		// started to the claim, so its files would otherwise come back
		// every minute until the user restarts the workspace. The delay
		// grows with the time the agent has been away instead.
		failure.quiet = true
		if agent.LastConnectedAt.Valid {
			failure.retryAfter = min(max(now.Sub(agent.LastConnectedAt.Time), workspaceFileDeliveryRetryDelay), workspaceFileDeliveryMaxDisconnectedDelay)
		}
		return workspacesdk.AgentUploadChatFileResponse{}, failure
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

	// No chat turn heartbeat may keep the workspace alive during the
	// delivery, so it records usage itself; otherwise the workspace could
	// autostop during a long copy.
	uploadCtx, cancelUpload := context.WithCancelCause(ctx)
	defer cancelUpload(nil)
	stall := w.opts.Clock.AfterFunc(workspaceFileDeliveryStallTimeout, func() {
		cancelUpload(errWorkspaceFileDeliveryStalled)
	}, "chatworker", "workspace-file-delivery-stall")
	defer stall.Stop("chatworker", "workspace-file-delivery-stall")
	body := &workspaceFileUsageReader{
		r:     &workspaceFileStallReader{r: bytes.NewReader(file.Data), timer: stall},
		clock: w.opts.Clock,
		report: func() {
			if w.server.usageTracker != nil {
				w.server.usageTracker.Add(row.WorkspaceID.UUID)
			}
			workspacestats.ActivityBumpWorkspace(ctx, w.opts.Logger.Named("activity_bump"), w.opts.Store, row.WorkspaceID.UUID, time.Time{}, workspacestats.ActivityBumpReasonChatWorkspaceUpload)
		},
	}
	body.reportNow()
	resp, err := conn.UploadChatFile(uploadCtx, workspacesdk.UploadChatFileRequest{
		ChatID: chat.ID.String(),
		Name:   file.Name,
		FileID: file.ID,
		Body:   body,
	})
	if err != nil {
		if errors.Is(context.Cause(uploadCtx), errWorkspaceFileDeliveryStalled) {
			failure := retryWorkspaceFileDelivery(errWorkspaceFileDeliveryStalled)
			failure.cause = err
			return workspacesdk.AgentUploadChatFileResponse{}, failure
		}
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

// workspaceFileUsageReader reports workspace usage when a delivery starts
// and at most every workspaceFileDeliveryUsageInterval while its content
// streams.
type workspaceFileUsageReader struct {
	r          io.Reader
	clock      quartz.Clock
	report     func()
	lastReport time.Time
}

func (u *workspaceFileUsageReader) reportNow() {
	u.lastReport = u.clock.Now("chatworker", "workspace-file-delivery")
	u.report()
}

func (u *workspaceFileUsageReader) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	if n > 0 && u.clock.Since(u.lastReport, "chatworker", "workspace-file-delivery") >= workspaceFileDeliveryUsageInterval {
		u.reportNow()
	}
	return n, err
}

// workspaceFileStallReader restarts the stall timer each time the agent
// takes more of the body. The last read does not restart it, so the timer
// also bounds the wait for the agent's answer.
type workspaceFileStallReader struct {
	r     io.Reader
	timer *quartz.Timer
}

func (s *workspaceFileStallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err == nil {
		s.timer.Reset(workspaceFileDeliveryStallTimeout, "chatworker", "workspace-file-delivery-stall")
	}
	return n, err
}

// classifyWorkspaceFileUploadError parks a file the agent rejected, since
// repeating the same request cannot succeed, and retries server and
// transport errors. The stored error leaves out the agent's request URL,
// which the full error in the log keeps.
func classifyWorkspaceFileUploadError(err error) *workspaceFileDeliveryFailure {
	failure := classifyWorkspaceFileUploadErrorText(err)
	failure.cause = err
	return failure
}

func classifyWorkspaceFileUploadErrorText(err error) *workspaceFileDeliveryFailure {
	var sdkErr *codersdk.Error
	if !errors.As(err, &sdkErr) {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return retryWorkspaceFileDelivery(xerrors.Errorf("upload to the workspace agent: %w", err))
	}
	status := sdkErr.StatusCode()
	reason := sdkErr.Message
	if sdkErr.Detail != "" {
		reason = fmt.Sprintf("%s: %s", reason, sdkErr.Detail)
	}
	if status < http.StatusBadRequest || status >= http.StatusInternalServerError {
		return retryWorkspaceFileDelivery(xerrors.Errorf("the workspace agent failed the upload with status %d (%s)", status, reason))
	}
	// Agents that predate write-once uploads answer 400 to the file_id
	// parameter, and agents that predate chat uploads answer 404.
	if status == http.StatusBadRequest || status == http.StatusNotFound {
		return parkWorkspaceFileDelivery(xerrors.Errorf("the workspace agent rejected the upload with status %d (%s); update the workspace agent, for example by restarting the workspace, then retry", status, reason))
	}
	return parkWorkspaceFileDelivery(xerrors.Errorf("the workspace agent rejected the upload with status %d (%s)", status, reason))
}
