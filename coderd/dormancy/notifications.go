package dormancy

import (
	"context"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications"
)

type WorkspaceDormantNotification struct {
	Workspace   database.Workspace
	InitiatedBy string
	Reason      string
	CreatedBy   string
}

func NotifyWorkspaceDormant(
	ctx context.Context,
	logger slog.Logger,
	enqueuer notifications.Enqueuer,
	notification WorkspaceDormantNotification,
) {
	labels := map[string]string{
		"name":        notification.Workspace.Name,
		"initiatedBy": notification.InitiatedBy,
		"reason":      notification.Reason,
	}
	if _, err := enqueuer.Enqueue(
		ctx,
		notification.Workspace.OwnerID,
		notifications.TemplateWorkspaceDormant,
		labels,
		notification.CreatedBy,
		// Associate this notification with all the related entities.
		notification.Workspace.ID,
		notification.Workspace.OwnerID,
		notification.Workspace.TemplateID,
		notification.Workspace.OrganizationID,
	); err != nil {
		logger.Warn(ctx, "failed to notify of workspace marked as dormant", slog.Error(err))
	}
}
