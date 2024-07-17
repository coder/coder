package dormancy

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications"
)

type WorkspaceDormantNotification struct {
	Workspace database.Workspace
	Initiator string
	Reason    string
	CreatedBy string
}

func NotifyWorkspaceDormant(
	ctx context.Context,
	enqueuer notifications.Enqueuer,
	notification WorkspaceDormantNotification,
) (id *uuid.UUID, err error) {
	labels := map[string]string{
		"name":      notification.Workspace.Name,
		"initiator": notification.Initiator,
		"reason":    notification.Reason,
	}
	return enqueuer.Enqueue(
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
	)
}
