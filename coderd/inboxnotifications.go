package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/alerts"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/pubsub"
	markdown "github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

const (
	notificationFormatMarkdown  = "markdown"
	notificationFormatPlaintext = "plaintext"
)

var fallbackIcons = map[uuid.UUID]string{
	// workspace related notifications
	alerts.TemplateWorkspaceCreated:           codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceManuallyUpdated:   codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceDeleted:           codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceAutobuildFailed:   codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceDormant:           codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceAutoUpdated:       codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceMarkedForDeletion: codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceManualBuildFailed: codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceOutOfMemory:       codersdk.InboxAlertFallbackIconWorkspace,
	alerts.TemplateWorkspaceOutOfDisk:         codersdk.InboxAlertFallbackIconWorkspace,

	// account related notifications
	alerts.TemplateUserAccountCreated:           codersdk.InboxAlertFallbackIconAccount,
	alerts.TemplateUserAccountDeleted:           codersdk.InboxAlertFallbackIconAccount,
	alerts.TemplateUserAccountSuspended:         codersdk.InboxAlertFallbackIconAccount,
	alerts.TemplateUserAccountActivated:         codersdk.InboxAlertFallbackIconAccount,
	alerts.TemplateYourAccountSuspended:         codersdk.InboxAlertFallbackIconAccount,
	alerts.TemplateYourAccountActivated:         codersdk.InboxAlertFallbackIconAccount,
	alerts.TemplateUserRequestedOneTimePasscode: codersdk.InboxAlertFallbackIconAccount,

	// template related notifications
	alerts.TemplateTemplateDeleted:             codersdk.InboxAlertFallbackIconTemplate,
	alerts.TemplateTemplateDeprecated:          codersdk.InboxAlertFallbackIconTemplate,
	alerts.TemplateWorkspaceBuildsFailedReport: codersdk.InboxAlertFallbackIconTemplate,
}

func ensureNotificationIcon(notif codersdk.InboxAlert) codersdk.InboxAlert {
	if notif.Icon != "" {
		return notif
	}

	fallbackIcon, ok := fallbackIcons[notif.TemplateID]
	if !ok {
		fallbackIcon = codersdk.InboxAlertFallbackIconOther
	}

	notif.Icon = fallbackIcon
	return notif
}

// convertInboxAlertResponse works as a util function to transform a database.InboxAlert to codersdk.InboxAlert
func convertInboxAlertResponse(ctx context.Context, logger slog.Logger, notif database.InboxAlert) codersdk.InboxAlert {
	convertedNotif := codersdk.InboxAlert{
		ID:         notif.ID,
		UserID:     notif.UserID,
		TemplateID: notif.TemplateID,
		Targets:    notif.Targets,
		Title:      notif.Title,
		Content:    notif.Content,
		Icon:       notif.Icon,
		Actions: func() []codersdk.InboxAlertAction {
			var actionsList []codersdk.InboxAlertAction
			err := json.Unmarshal([]byte(notif.Actions), &actionsList)
			if err != nil {
				logger.Error(ctx, "unmarshal inbox notification actions", slog.Error(err))
			}
			return actionsList
		}(),
		ReadAt: func() *time.Time {
			if !notif.ReadAt.Valid {
				return nil
			}
			return &notif.ReadAt.Time
		}(),
		CreatedAt: notif.CreatedAt,
	}

	return ensureNotificationIcon(convertedNotif)
}

// watchInboxAlerts watches for new inbox notifications and sends them to the client.
// The client can specify a list of target IDs to filter the notifications.
// @Summary Watch for new inbox notifications
// @ID watch-for-new-inbox-notifications
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Param targets query string false "Comma-separated list of target IDs to filter notifications"
// @Param templates query string false "Comma-separated list of template IDs to filter notifications"
// @Param read_status query string false "Filter notifications by read status. Possible values: read, unread, all"
// @Param format query string false "Define the output format for notifications title and body." enums(plaintext,markdown)
// @Success 200 {object} codersdk.GetInboxAlertResponse
// @Router /notifications/inbox/watch [get]
func (api *API) watchInboxAlerts(rw http.ResponseWriter, r *http.Request) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)

		targets    = p.UUIDs(vals, []uuid.UUID{}, "targets")
		templates  = p.UUIDs(vals, []uuid.UUID{}, "templates")
		readStatus = p.String(vals, "all", "read_status")
		format     = p.String(vals, notificationFormatMarkdown, "format")
	)
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	if !slices.Contains([]string{
		string(database.InboxAlertReadStatusAll),
		string(database.InboxAlertReadStatusRead),
		string(database.InboxAlertReadStatusUnread),
	}, readStatus) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "starting_before query parameter should be any of 'all', 'read', 'unread'.",
		})
		return
	}

	notificationCh := make(chan codersdk.InboxAlert, 10)

	closeInboxAlertsSubscriber, err := api.Pubsub.SubscribeWithErr(pubsub.InboxAlertForOwnerEventChannel(apikey.UserID),
		pubsub.HandleInboxAlertEvent(
			func(ctx context.Context, payload pubsub.InboxAlertEvent, err error) {
				if err != nil {
					api.Logger.Error(ctx, "inbox notification event", slog.Error(err))
					return
				}

				// HandleInboxAlertEvent cb receives all the inbox notifications - without any filters excepted the user_id.
				// Based on query parameters defined above and filters defined by the client - we then filter out the
				// notifications we do not want to forward and discard it.

				// filter out notifications that don't match the targets
				if len(targets) > 0 {
					for _, target := range targets {
						if isFound := slices.Contains(payload.InboxAlert.Targets, target); !isFound {
							return
						}
					}
				}

				// filter out notifications that don't match the templates
				if len(templates) > 0 {
					if isFound := slices.Contains(templates, payload.InboxAlert.TemplateID); !isFound {
						return
					}
				}

				// filter out notifications that don't match the read status
				if readStatus != "" {
					if readStatus == string(database.InboxAlertReadStatusRead) {
						if payload.InboxAlert.ReadAt == nil {
							return
						}
					} else if readStatus == string(database.InboxAlertReadStatusUnread) {
						if payload.InboxAlert.ReadAt != nil {
							return
						}
					}
				}

				// keep a safe guard in case of latency to push notifications through websocket
				select {
				case notificationCh <- ensureNotificationIcon(payload.InboxAlert):
				default:
					api.Logger.Error(ctx, "failed to push consumed notification into websocket handler, check latency")
				}
			},
		))
	if err != nil {
		api.Logger.Error(ctx, "subscribe to inbox notification event", slog.Error(err))
		return
	}
	defer closeInboxAlertsSubscriber()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upgrade connection to websocket.",
			Detail:  err.Error(),
		})
		return
	}

	go httpapi.Heartbeat(ctx, conn)
	defer conn.Close(websocket.StatusNormalClosure, "connection closed")

	encoder := wsjson.NewEncoder[codersdk.GetInboxAlertResponse](conn, websocket.MessageText)
	defer encoder.Close(websocket.StatusNormalClosure)

	// Log the request immediately instead of after it completes.
	if rl := loggermw.RequestLoggerFromContext(ctx); rl != nil {
		rl.WriteLog(ctx, http.StatusAccepted)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case notif := <-notificationCh:
			unreadCount, err := api.Database.CountUnreadInboxAlertsByUserID(ctx, apikey.UserID)
			if err != nil {
				api.Logger.Error(ctx, "failed to count unread inbox notifications", slog.Error(err))
				return
			}

			// By default, notifications are stored as markdown
			// We can change the format based on parameter if required
			if format == notificationFormatPlaintext {
				notif.Title, err = markdown.PlaintextFromMarkdown(notif.Title)
				if err != nil {
					api.Logger.Error(ctx, "failed to convert notification title to plain text", slog.Error(err))
					return
				}

				notif.Content, err = markdown.PlaintextFromMarkdown(notif.Content)
				if err != nil {
					api.Logger.Error(ctx, "failed to convert notification content to plain text", slog.Error(err))
					return
				}
			}

			if err := encoder.Encode(codersdk.GetInboxAlertResponse{
				Notification: notif,
				UnreadCount:  int(unreadCount),
			}); err != nil {
				api.Logger.Error(ctx, "encode notification", slog.Error(err))
				return
			}
		}
	}
}

// listInboxAlerts lists the notifications for the user.
// @Summary List inbox notifications
// @ID list-inbox-notifications
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Param targets query string false "Comma-separated list of target IDs to filter notifications"
// @Param templates query string false "Comma-separated list of template IDs to filter notifications"
// @Param read_status query string false "Filter notifications by read status. Possible values: read, unread, all"
// @Param starting_before query string false "ID of the last notification from the current page. Notifications returned will be older than the associated one" format(uuid)
// @Success 200 {object} codersdk.ListInboxAlertsResponse
// @Router /notifications/inbox [get]
func (api *API) listInboxAlerts(rw http.ResponseWriter, r *http.Request) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)

		targets        = p.UUIDs(vals, nil, "targets")
		templates      = p.UUIDs(vals, nil, "templates")
		readStatus     = p.String(vals, "all", "read_status")
		startingBefore = p.UUID(vals, uuid.Nil, "starting_before")
	)
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	if !slices.Contains([]string{
		string(database.InboxAlertReadStatusAll),
		string(database.InboxAlertReadStatusRead),
		string(database.InboxAlertReadStatusUnread),
	}, readStatus) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "starting_before query parameter should be any of 'all', 'read', 'unread'.",
		})
		return
	}

	createdBefore := dbtime.Now()
	if startingBefore != uuid.Nil {
		lastNotif, err := api.Database.GetInboxAlertByID(ctx, startingBefore)
		if err == nil {
			createdBefore = lastNotif.CreatedAt
		}
	}

	notifs, err := api.Database.GetFilteredInboxAlertsByUserID(ctx, database.GetFilteredInboxAlertsByUserIDParams{
		UserID:       apikey.UserID,
		Templates:    templates,
		Targets:      targets,
		ReadStatus:   database.InboxAlertReadStatus(readStatus),
		CreatedAtOpt: createdBefore,
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to get filtered inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get filtered inbox notifications.",
		})
		return
	}

	unreadCount, err := api.Database.CountUnreadInboxAlertsByUserID(ctx, apikey.UserID)
	if err != nil {
		api.Logger.Error(ctx, "failed to count unread inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to count unread inbox notifications.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ListInboxAlertsResponse{
		Notifications: func() []codersdk.InboxAlert {
			notificationsList := make([]codersdk.InboxAlert, 0, len(notifs))
			for _, notification := range notifs {
				notificationsList = append(notificationsList, convertInboxAlertResponse(ctx, api.Logger, notification))
			}
			return notificationsList
		}(),
		UnreadCount: int(unreadCount),
	})
}

// updateInboxAlertReadStatus changes the read status of a notification.
// @Summary Update read status of a notification
// @ID update-read-status-of-a-notification
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Param id path string true "id of the notification"
// @Success 200 {object} codersdk.Response
// @Router /notifications/inbox/{id}/read-status [put]
func (api *API) updateInboxAlertReadStatus(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)
	)

	notificationID, ok := httpmw.ParseUUIDParam(rw, r, "id")
	if !ok {
		return
	}

	var body codersdk.UpdateInboxAlertReadStatusRequest
	if !httpapi.Read(ctx, rw, r, &body) {
		return
	}

	err := api.Database.UpdateInboxAlertReadStatus(ctx, database.UpdateInboxAlertReadStatusParams{
		ID: notificationID,
		ReadAt: func() sql.NullTime {
			if body.IsRead {
				return sql.NullTime{
					Time:  dbtime.Now(),
					Valid: true,
				}
			}

			return sql.NullTime{}
		}(),
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to update inbox notification read status", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update inbox notification read status.",
		})
		return
	}

	unreadCount, err := api.Database.CountUnreadInboxAlertsByUserID(ctx, apikey.UserID)
	if err != nil {
		api.Logger.Error(ctx, "failed to call count unread inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to call count unread inbox notifications.",
		})
		return
	}

	updatedNotification, err := api.Database.GetInboxAlertByID(ctx, notificationID)
	if err != nil {
		api.Logger.Error(ctx, "failed to get notification by id", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get notification by id.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UpdateInboxAlertReadStatusResponse{
		Notification: convertInboxAlertResponse(ctx, api.Logger, updatedNotification),
		UnreadCount:  int(unreadCount),
	})
}

// markAllInboxAlertsAsRead marks as read all unread notifications for authenticated user.
// @Summary Mark all unread notifications as read
// @ID mark-all-unread-notifications-as-read
// @Security CoderSessionToken
// @Tags Notifications
// @Success 204
// @Router /notifications/inbox/mark-all-as-read [put]
func (api *API) markAllInboxAlertsAsRead(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)
	)

	err := api.Database.MarkAllInboxAlertsAsRead(ctx, database.MarkAllInboxAlertsAsReadParams{
		UserID: apikey.UserID,
		ReadAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to mark all unread notifications as read", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to mark all unread notifications as read.",
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
