package system

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/types"
)

// NotifyWorkspaceDeleted notifies the given user that their workspace was deleted.
func NotifyWorkspaceDeleted(ctx context.Context, userID uuid.UUID, name, reason, createdBy string, targets ...uuid.UUID) {
	_, _ = notifications.Enqueue(ctx, userID, notifications.TemplateWorkspaceDeleted,
		types.Labels{
			"name":   name,
			"reason": reason,
		}, createdBy, targets...)
}
