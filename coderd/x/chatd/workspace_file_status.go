package chatd

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

const (
	workspaceFileStatusPollInterval = time.Second
	workspaceFileStatusMaxWait      = 2 * time.Minute
)

// workspaceFilesAwaitMode selects whether awaitChatWorkspaceFiles first
// makes failed files due again.
type workspaceFilesAwaitMode int

const (
	// workspaceFilesReport waits for and reports the files as they are.
	workspaceFilesReport workspaceFilesAwaitMode = iota
	// workspaceFilesRetry resets failed files before waiting.
	workspaceFilesRetry
)

// awaitChatWorkspaceFiles pokes the delivery loop, waits until none of
// the chat's workspace attachments is due or in flight when the chat's
// workspace finished starting, and reports every workspace attachment in
// the chat's user messages. It never delivers a file itself, so the
// delivery loop stays the only sender. Errors are reported in the result
// so a caller never fails because of an attachment.
func (p *Server) awaitChatWorkspaceFiles(ctx context.Context, chatID uuid.UUID, mode workspaceFilesAwaitMode) chattool.WorkspaceAttachmentsReport {
	var errs []error
	if mode == workspaceFilesRetry {
		if _, err := p.db.RetryChatWorkspaceFileDeliveries(ctx, chatID); err != nil {
			errs = append(errs, xerrors.Errorf("retry workspace attachments: %w", err))
		}
	}
	p.wakeWorkspaceFileDelivery()

	parts, err := p.chatWorkspaceAttachmentParts(ctx, chatID)
	if err != nil {
		errs = append(errs, err)
		return chattool.WorkspaceAttachmentsReport{Err: errors.Join(errs...)}
	}
	if len(parts) == 0 {
		return chattool.WorkspaceAttachmentsReport{Err: errors.Join(errs...)}
	}

	startupFinished, err := p.chatWorkspaceStartupFinished(ctx, chatID)
	if err != nil {
		errs = append(errs, err)
	}
	fileIDs := make(map[uuid.UUID]struct{}, len(parts))
	for _, part := range parts {
		fileIDs[part.FileID.UUID] = struct{}{}
	}
	files, err := p.db.GetChatFileMetadataByChatID(ctx, chatID)
	if err != nil {
		errs = append(errs, xerrors.Errorf("read workspace attachments: %w", err))
		return chattool.WorkspaceAttachmentsReport{Err: errors.Join(errs...)}
	}
	// Nothing is copied before startup finishes, so there is nothing to
	// wait for.
	if startupFinished && p.workspaceFilesOutstanding(files, fileIDs) {
		files, err = p.waitForChatWorkspaceFiles(ctx, chatID, fileIDs, files)
		if err != nil {
			errs = append(errs, xerrors.Errorf("wait for workspace attachments: %w", err))
		}
	}

	undelivered := chattool.WorkspaceAttachmentPending
	if startupFinished {
		undelivered = chattool.WorkspaceAttachmentCopying
	}
	byID := make(map[uuid.UUID]*database.GetChatFileMetadataByChatIDRow, len(files))
	for i := range files {
		byID[files[i].ID] = &files[i]
	}
	attachments := make([]chattool.WorkspaceAttachment, 0, len(parts))
	for _, part := range parts {
		attachments = append(attachments, workspaceAttachmentStatus(part, byID[part.FileID.UUID], undelivered))
	}
	return chattool.WorkspaceAttachmentsReport{Attachments: attachments, Err: errors.Join(errs...)}
}

// chatWorkspaceAttachmentParts returns the chat's workspace-attachment
// parts from user messages, one per file. It reads every message, not
// only those after the last compaction, because compaction keeps the
// original rows.
func (p *Server) chatWorkspaceAttachmentParts(ctx context.Context, chatID uuid.UUID) ([]codersdk.ChatMessagePart, error) {
	messages, err := p.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	if err != nil {
		return nil, xerrors.Errorf("read chat messages: %w", err)
	}
	var parts []codersdk.ChatMessagePart
	seen := make(map[uuid.UUID]struct{})
	for _, msg := range messages {
		if msg.Role != database.ChatMessageRoleUser {
			continue
		}
		msgParts, err := chatprompt.ParseContent(msg)
		if err != nil {
			return nil, xerrors.Errorf("parse chat message %d: %w", msg.ID, err)
		}
		for _, part := range msgParts {
			if part.Type != codersdk.ChatMessagePartTypeWorkspaceAttachment || !part.FileID.Valid {
				continue
			}
			if _, ok := seen[part.FileID.UUID]; ok {
				continue
			}
			seen[part.FileID.UUID] = struct{}{}
			parts = append(parts, part)
		}
	}
	return parts, nil
}

// chatWorkspaceStartupFinished reports whether the chat's workspace
// passes the condition AcquireChatWorkspaceFileDeliveries claims files
// under: the latest build is a start build and the chat's agent finished
// its startup scripts.
func (p *Server) chatWorkspaceStartupFinished(ctx context.Context, chatID uuid.UUID) (bool, error) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return false, xerrors.Errorf("load chat: %w", err)
	}
	if !chat.WorkspaceID.Valid {
		return false, nil
	}
	build, err := p.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, chat.WorkspaceID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, xerrors.Errorf("load latest workspace build: %w", err)
	}
	if build.Transition != database.WorkspaceTransitionStart {
		return false, nil
	}
	agents, err := p.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		return false, xerrors.Errorf("load workspace agents: %w", err)
	}
	agent, err := agentselect.ForChat(chat, agents)
	if err != nil {
		// The delivery loop parks files it cannot pick an agent for, so
		// the error reaches the model through the file's status.
		p.logger.Debug(ctx, "select the chat's workspace agent for attachments",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return false, nil
	}
	return agent.LifecycleState.StartupFinished(), nil
}

// waitForChatWorkspaceFiles polls the chat's files until none of fileIDs
// is due or in flight, or the wait ends, and returns the last read.
func (p *Server) waitForChatWorkspaceFiles(
	ctx context.Context,
	chatID uuid.UUID,
	fileIDs map[uuid.UUID]struct{},
	files []database.GetChatFileMetadataByChatIDRow,
) ([]database.GetChatFileMetadataByChatIDRow, error) {
	timer := p.clock.NewTimer(workspaceFileStatusMaxWait, "chatd", "await-workspace-files")
	defer timer.Stop("chatd", "await-workspace-files")
	ticker := p.clock.NewTicker(workspaceFileStatusPollInterval, "chatd", "await-workspace-files")
	defer ticker.Stop("chatd", "await-workspace-files")
	for {
		select {
		case <-ctx.Done():
			return files, ctx.Err()
		case <-timer.C:
			return files, nil
		case <-ticker.C:
		}
		latest, err := p.db.GetChatFileMetadataByChatID(ctx, chatID)
		if err != nil {
			return files, err
		}
		files = latest
		if !p.workspaceFilesOutstanding(files, fileIDs) {
			return files, nil
		}
	}
}

// workspaceFilesOutstanding reports whether any of fileIDs is due or in
// flight. A claim counts the attempt, clears the last error and leases
// the file, so a future next attempt without an error is in flight, even
// on the last attempt, and one with an error is waiting to retry.
func (p *Server) workspaceFilesOutstanding(files []database.GetChatFileMetadataByChatIDRow, fileIDs map[uuid.UUID]struct{}) bool {
	now := p.clock.Now("chatd", "await-workspace-files")
	for _, file := range files {
		if _, ok := fileIDs[file.ID]; !ok {
			continue
		}
		if file.Purpose != database.ChatFilePurposeWorkspace || file.WorkspaceDeliveredAt.Valid {
			continue
		}
		scheduled := file.WorkspaceDeliveryNextAttemptAt.Valid &&
			file.WorkspaceDeliveryNextAttemptAt.Time.After(now)
		inFlight := scheduled && !file.WorkspaceDeliveryError.Valid
		due := !scheduled && file.WorkspaceDeliveryAttempts < database.ChatWorkspaceFileMaxDeliveryAttempts
		if inFlight || due {
			return true
		}
	}
	return false
}

// workspaceAttachmentStatus reports the status of the file that part
// refers to. file is nil when the file is no longer stored. undelivered
// is the status of a file that is neither copied nor failed.
func workspaceAttachmentStatus(
	part codersdk.ChatMessagePart,
	file *database.GetChatFileMetadataByChatIDRow,
	undelivered chattool.WorkspaceAttachmentStatus,
) chattool.WorkspaceAttachment {
	attachment := chattool.WorkspaceAttachment{
		FileID:    part.FileID.UUID.String(),
		Name:      part.Name,
		MediaType: part.MediaType,
		SizeBytes: part.Size,
	}
	if file == nil || file.Purpose != database.ChatFilePurposeWorkspace {
		attachment.Status = chattool.WorkspaceAttachmentUnavailable
		return attachment
	}
	attachment.Name = file.Name
	attachment.MediaType = file.Mimetype
	attachment.SizeBytes = file.SizeBytes
	switch {
	case file.WorkspaceDeliveredAt.Valid:
		attachment.Status = chattool.WorkspaceAttachmentCopied
		attachment.Path = file.WorkspaceDeliveredPath.String
		if file.WorkspaceDeliveredWorkspaceID.Valid {
			attachment.WorkspaceID = file.WorkspaceDeliveredWorkspaceID.UUID.String()
		}
	case file.WorkspaceDeliveryError.Valid:
		attachment.Status = chattool.WorkspaceAttachmentFailed
		attachment.Error = file.WorkspaceDeliveryError.String
	default:
		attachment.Status = undelivered
	}
	return attachment
}
