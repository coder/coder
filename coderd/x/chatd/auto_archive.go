package chatd

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

const chatAutoArchiveDigestMaxChats = 25

type autoArchivedChat struct {
	Chat           database.Chat
	LastActivityAt time.Time
}

func (w *chatWorker) archiveLoop(ctx context.Context) {
	run := func(start time.Time) {
		w.archiveOnce(ctx, dbtime.Time(start).UTC())
	}
	run(w.opts.Clock.Now("chatworker", "auto-archive"))

	ticker := w.opts.Clock.NewTicker(w.opts.ArchiveInterval, "chatworker", "auto-archive")
	defer ticker.Stop("chatworker", "auto-archive")
	for {
		select {
		case tick := <-ticker.C:
			ticker.Stop("chatworker", "auto-archive")
			run(tick)
			ticker.Reset(w.opts.ArchiveInterval, "chatworker", "auto-archive")
		case <-ctx.Done():
			return
		}
	}
}

func (w *chatWorker) archiveOnce(ctx context.Context, start time.Time) {
	autoArchiveDays, err := w.opts.Store.GetChatAutoArchiveDays(ctx, codersdk.DefaultChatAutoArchiveDays)
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker auto-archive config read failed", slogError(err))
		}
		return
	}
	if autoArchiveDays <= 0 {
		return
	}
	retentionDays, err := w.opts.Store.GetChatRetentionDays(ctx)
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker chat retention config read failed", slogError(err))
		}
		return
	}

	// Anchor the cutoff at 00:00 UTC so every chat with activity on the
	// same UTC calendar date stays eligible or ineligible for the whole
	// day. This avoids trickling chats into auto-archive as wall-clock
	// time advances.
	archiveCutoff := dbtime.StartOfDay(start.UTC()).Add(-time.Duration(autoArchiveDays) * 24 * time.Hour)
	rows, err := w.opts.Store.GetAutoArchiveInactiveChatCandidates(ctx, database.GetAutoArchiveInactiveChatCandidatesParams{
		ArchiveCutoff: archiveCutoff,
		LimitCount:    w.opts.ArchiveBatchSize,
	})
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker auto-archive query failed", slogError(err))
		}
		return
	}
	if len(rows) == 0 {
		return
	}

	archived := make([]autoArchivedChat, 0, len(rows))
	for _, row := range rows {
		family, err := w.archiveCandidateSafely(ctx, row)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if isExpectedAutoArchiveError(err) {
				w.opts.Logger.Debug(ctx, "chatworker auto-archive skipped chat",
					slog.F("chat_id", row.ID),
					slog.Error(err),
				)
				continue
			}
			w.opts.Logger.Warn(ctx, "chatworker auto-archive candidate failed",
				slog.F("chat_id", row.ID),
				slog.Error(err),
			)
			continue
		}
		archived = append(archived, family...)
	}
	if len(archived) == 0 {
		return
	}
	if w.opts.AutoArchiveRecords != nil {
		w.opts.AutoArchiveRecords.Add(float64(len(archived)))
	}
	w.dispatchChatAutoArchive(context.WithoutCancel(ctx), ctx, start, autoArchiveDays, retentionDays, archived)
}

func (w *chatWorker) archiveCandidateSafely(
	ctx context.Context,
	row database.GetAutoArchiveInactiveChatCandidatesRow,
) (family []autoArchivedChat, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = xerrors.Errorf("chatworker auto-archive panic: %v", recovered)
		}
	}()
	return w.archiveCandidate(ctx, row)
}

func (w *chatWorker) archiveCandidate(
	ctx context.Context,
	row database.GetAutoArchiveInactiveChatCandidatesRow,
) ([]autoArchivedChat, error) {
	familyChats, err := chatstate.SetFamilyArchived(ctx, w.opts.Store, w.opts.Pubsub, chatstate.SetFamilyArchivedInput{
		RootID:   row.ID,
		Archived: true,
	})
	if err != nil {
		return nil, err
	}
	if len(familyChats) == 0 {
		return nil, nil
	}
	w.server.scheduleArchiveDebugCleanup(ctx, familyChats)
	w.server.publishChatPubsubEvents(familyChats, codersdk.ChatWatchEventKindDeleted)

	archived := make([]autoArchivedChat, 0, len(familyChats))
	for _, chat := range familyChats {
		lastActivityAt := row.LastActivityAt
		if lastActivityAt.IsZero() {
			lastActivityAt = chat.CreatedAt
		}
		archived = append(archived, autoArchivedChat{
			Chat:           chat,
			LastActivityAt: lastActivityAt,
		})
	}
	return archived, nil
}

func isExpectedAutoArchiveError(err error) bool {
	return errors.Is(err, sql.ErrNoRows) ||
		errors.Is(err, chatstate.ErrChatNotFound) ||
		errors.Is(err, chatstate.ErrChatNotRoot) ||
		errors.Is(err, chatstate.ErrInvalidState) ||
		errors.Is(err, chatstate.ErrTransitionNotAllowed)
}

func (p *Server) scheduleArchiveDebugCleanup(ctx context.Context, familyChats []database.Chat) {
	if len(familyChats) == 0 {
		return
	}
	archiveCutoff := familyChats[0].UpdatedAt.Add(-debugCleanupClockSkew)
	for _, archivedChat := range familyChats {
		p.scheduleDebugCleanup(
			ctx,
			"failed to delete chat debug rows after archive",
			[]slog.Field{slog.F("chat_id", archivedChat.ID)},
			func(cleanupCtx context.Context, debugSvc *chatdebug.Service) error {
				_, err := debugSvc.DeleteByChatID(cleanupCtx, archivedChat.ID, archiveCutoff)
				return err
			},
		)
	}
}

func (w *chatWorker) dispatchChatAutoArchive(
	auditCtx context.Context,
	enqueueCtx context.Context,
	tickStart time.Time,
	autoArchiveDays int32,
	retentionDays int32,
	archived []autoArchivedChat,
) {
	roots := make([]autoArchivedChat, 0, len(archived))
	for _, record := range archived {
		if !record.Chat.ParentChatID.Valid {
			roots = append(roots, record)
		}
	}
	w.auditAutoArchivedChats(auditCtx, roots)
	w.enqueueAutoArchiveDigests(enqueueCtx, tickStart, autoArchiveDays, retentionDays, roots)
}

func (w *chatWorker) auditAutoArchivedChats(ctx context.Context, roots []autoArchivedChat) {
	if w.opts.Auditor == nil {
		return
	}
	auditor := w.opts.Auditor.Load()
	if auditor == nil {
		return
	}
	for _, record := range roots {
		after := record.Chat
		before := after
		before.Archived = false
		audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.Chat]{
			Audit:            *auditor,
			Log:              w.opts.Logger,
			UserID:           after.OwnerID,
			OrganizationID:   after.OrganizationID,
			Action:           database.AuditActionWrite,
			Old:              before,
			New:              after,
			Status:           http.StatusOK,
			AdditionalFields: audit.BackgroundTaskFieldsBytes(ctx, w.opts.Logger, audit.BackgroundSubsystemChatAutoArchive),
		})
	}
}

func (w *chatWorker) enqueueAutoArchiveDigests(
	ctx context.Context,
	tickStart time.Time,
	autoArchiveDays int32,
	retentionDays int32,
	roots []autoArchivedChat,
) {
	rootsByOwner := make(map[uuid.UUID][]autoArchivedChat, len(roots))
	for _, record := range roots {
		rootsByOwner[record.Chat.OwnerID] = append(rootsByOwner[record.Chat.OwnerID], record)
	}
	ownerIDs := make([]uuid.UUID, 0, len(rootsByOwner))
	for id := range rootsByOwner {
		ownerIDs = append(ownerIDs, id)
	}
	slices.SortFunc(ownerIDs, func(a, b uuid.UUID) int {
		return cmp.Compare(a.String(), b.String())
	})
	for i, ownerID := range ownerIDs {
		if err := ctx.Err(); err != nil {
			w.opts.Logger.Warn(ctx, "chat auto-archive digest dispatch canceled",
				slog.F("remaining_owners", len(ownerIDs)-i),
				slog.Error(err),
			)
			return
		}
		data := buildAutoArchiveDigestData(rootsByOwner[ownerID], autoArchiveDays, retentionDays, tickStart)
		//nolint:gocritic // Background digest dispatch runs as the notifier subject.
		if _, err := w.opts.NotificationsEnqueuer.EnqueueWithData(
			dbauthz.AsNotifier(ctx),
			ownerID,
			notifications.TemplateChatAutoArchiveDigest,
			map[string]string{},
			data,
			string(audit.BackgroundSubsystemChatAutoArchive),
		); err != nil {
			w.opts.Logger.Warn(ctx, "failed to enqueue chat auto-archive digest",
				slog.F("owner_id", ownerID),
				slog.Error(err),
			)
		}
	}
}

func buildAutoArchiveDigestData(rows []autoArchivedChat, autoArchiveDays, retentionDays int32, tickStart time.Time) map[string]any {
	overflow := 0
	if len(rows) > chatAutoArchiveDigestMaxChats {
		overflow = len(rows) - chatAutoArchiveDigestMaxChats
		rows = rows[:chatAutoArchiveDigestMaxChats]
	}
	chats := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		chats = append(chats, map[string]any{
			"title":                   r.Chat.Title,
			"last_activity_humanized": humanize.RelTime(r.LastActivityAt, tickStart, "ago", "from now"),
		})
	}
	data := map[string]any{
		"auto_archive_days": strconv.Itoa(int(autoArchiveDays)),
		"retention_days":    strconv.Itoa(int(retentionDays)),
		"archived_chats":    chats,
	}
	if overflow > 0 {
		data["additional_archived_count"] = strconv.Itoa(overflow)
	}
	return data
}
