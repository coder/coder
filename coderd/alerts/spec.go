package alerts

import (
	"context"
	"text/template"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/alerts/dispatch"
	"github.com/coder/coder/v2/coderd/alerts/types"
	"github.com/coder/coder/v2/coderd/database"
)

// Store defines the API between the notifications system and the storage.
// This abstraction is in place so that we can intercept the direct database interactions, or (later) swap out these calls
// with dRPC calls should we want to split the notifiers out into their own component for high availability/throughput.
// TODO: don't use database types here
type Store interface {
	AcquireAlertMessages(ctx context.Context, params database.AcquireAlertMessagesParams) ([]database.AcquireAlertMessagesRow, error)
	BulkMarkAlertMessagesSent(ctx context.Context, arg database.BulkMarkAlertMessagesSentParams) (int64, error)
	BulkMarkAlertMessagesFailed(ctx context.Context, arg database.BulkMarkAlertMessagesFailedParams) (int64, error)
	EnqueueAlertMessage(ctx context.Context, arg database.EnqueueAlertMessageParams) error
	FetchNewMessageMetadata(ctx context.Context, arg database.FetchNewMessageMetadataParams) (database.FetchNewMessageMetadataRow, error)
	GetAlertMessagesByStatus(ctx context.Context, arg database.GetAlertMessagesByStatusParams) ([]database.AlertMessage, error)
	GetNotificationsSettings(ctx context.Context) (string, error)
	GetApplicationName(ctx context.Context) (string, error)
	GetLogoURL(ctx context.Context) (string, error)

	InsertInboxAlert(ctx context.Context, arg database.InsertInboxAlertParams) (database.InboxAlert, error)
}

// Handler is responsible for preparing and delivering a notification by a given method.
type Handler interface {
	// Dispatcher constructs a DeliveryFunc to be used for delivering a notification via the chosen method.
	Dispatcher(payload types.MessagePayload, title, body string, helpers template.FuncMap) (dispatch.DeliveryFunc, error)
}

// Enqueuer enqueues a new notification message in the store and returns its ID, should it enqueue without failure.
type Enqueuer interface {
	Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error)
	EnqueueWithData(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, data map[string]any, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error)
}
