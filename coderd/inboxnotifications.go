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

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

// convertInboxNotificationResponse works as a util function to transform a database.InboxNotification to codersdk.InboxNotification
func convertInboxNotificationResponse(ctx context.Context, logger slog.Logger, notif database.InboxNotification) codersdk.InboxNotification {
	return codersdk.InboxNotification{
		ID:         notif.ID,
		UserID:     notif.UserID,
		TemplateID: notif.TemplateID,
		Targets:    notif.Targets,
		Title:      notif.Title,
		Content:    notif.Content,
		Icon:       notif.Icon,
		Actions: func() []codersdk.InboxNotificationAction {
			var actionsList []codersdk.InboxNotificationAction
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
}

// watchInboxNotifications watches for new inbox notifications and sends them to the client.
// The client can specify a list of target IDs to filter the notifications.
// @Summary Watch for new inbox notifications
// @ID watch-for-new-inbox-notifications
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Param targets query string false "Comma-separated list of target IDs to filter notifications"
// @Param templates query string false "Comma-separated list of template IDs to filter notifications"
// @Param read_status query string false "Filter notifications by read status. Possible values: read, unread, all"
// @Success 200 {object} codersdk.GetInboxNotificationResponse
// @Router /notifications/inbox/watch [get]
func (api *API) watchInboxNotifications(rw http.ResponseWriter, r *http.Request) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)

		targets    = p.UUIDs(vals, []uuid.UUID{}, "targets")
		templates  = p.UUIDs(vals, []uuid.UUID{}, "templates")
		readStatus = p.String(vals, "all", "read_status")
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
		string(database.InboxNotificationReadStatusAll),
		string(database.InboxNotificationReadStatusRead),
		string(database.InboxNotificationReadStatusUnread),
	}, readStatus) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "starting_before query parameter should be any of 'all', 'read', 'unread'.",
		})
		return
	}

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

	notificationCh := make(chan codersdk.InboxNotification, 10)

	closeInboxNotificationsSubscriber, err := api.Pubsub.SubscribeWithErr(pubsub.InboxNotificationForOwnerEventChannel(apikey.UserID),
		pubsub.HandleInboxNotificationEvent(
			func(ctx context.Context, payload pubsub.InboxNotificationEvent, err error) {
				if err != nil {
					api.Logger.Error(ctx, "inbox notification event", slog.Error(err))
					return
				}

				// HandleInboxNotificationEvent cb receives all the inbox notifications - without any filters excepted the user_id.
				// Based on query parameters defined above and filters defined by the client - we then filter out the
				// notifications we do not want to forward and discard it.

				// filter out notifications that don't match the targets
				if len(targets) > 0 {
					for _, target := range targets {
						if isFound := slices.Contains(payload.InboxNotification.Targets, target); !isFound {
							return
						}
					}
				}

				// filter out notifications that don't match the templates
				if len(templates) > 0 {
					if isFound := slices.Contains(templates, payload.InboxNotification.TemplateID); !isFound {
						return
					}
				}

				// filter out notifications that don't match the read status
				if readStatus != "" {
					if readStatus == string(database.InboxNotificationReadStatusRead) {
						if payload.InboxNotification.ReadAt == nil {
							return
						}
					} else if readStatus == string(database.InboxNotificationReadStatusUnread) {
						if payload.InboxNotification.ReadAt != nil {
							return
						}
					}
				}

				// keep a safe guard in case of latency to push notifications through websocket
				select {
				case notificationCh <- payload.InboxNotification:
				default:
				}
			},
		))
	if err != nil {
		api.Logger.Error(ctx, "subscribe to inbox notification event", slog.Error(err))
		return
	}

	defer closeInboxNotificationsSubscriber()

	encoder := wsjson.NewEncoder[codersdk.GetInboxNotificationResponse](conn, websocket.MessageText)
	defer encoder.Close(websocket.StatusNormalClosure)

	for {
		select {
		case <-ctx.Done():
			return
		case notif := <-notificationCh:
			unreadCount, err := api.Database.CountUnreadInboxNotificationsByUserID(ctx, apikey.UserID)
			if err != nil {
				api.Logger.Error(ctx, "failed to count unread inbox notifications", slog.Error(err))
				return
			}
			if err := encoder.Encode(codersdk.GetInboxNotificationResponse{
				Notification: notif,
				UnreadCount:  int(unreadCount),
			}); err != nil {
				api.Logger.Error(ctx, "encode notification", slog.Error(err))
				return
			}
		}
	}
}

// listInboxNotifications lists the notifications for the user.
// @Summary List inbox notifications
// @ID list-inbox-notifications
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Param targets query string false "Comma-separated list of target IDs to filter notifications"
// @Param templates query string false "Comma-separated list of template IDs to filter notifications"
// @Param read_status query string false "Filter notifications by read status. Possible values: read, unread, all"
// @Success 200 {object} codersdk.ListInboxNotificationsResponse
// @Router /notifications/inbox [get]
func (api *API) listInboxNotifications(rw http.ResponseWriter, r *http.Request) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)

		targets        = p.UUIDs(vals, []uuid.UUID{}, "targets")
		templates      = p.UUIDs(vals, []uuid.UUID{}, "templates")
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
		string(database.InboxNotificationReadStatusAll),
		string(database.InboxNotificationReadStatusRead),
		string(database.InboxNotificationReadStatusUnread),
	}, readStatus) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "starting_before query parameter should be any of 'all', 'read', 'unread'.",
		})
		return
	}

	createdBefore := dbtime.Now()
	if startingBefore != uuid.Nil {
		lastNotif, err := api.Database.GetInboxNotificationByID(ctx, startingBefore)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to get notification by id.",
			})
			return
		}
		createdBefore = lastNotif.CreatedAt
	}

	notifs, err := api.Database.GetFilteredInboxNotificationsByUserID(ctx, database.GetFilteredInboxNotificationsByUserIDParams{
		UserID:       apikey.UserID,
		Templates:    templates,
		Targets:      targets,
		ReadStatus:   database.InboxNotificationReadStatus(readStatus),
		CreatedAtOpt: createdBefore,
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to get filtered inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get filtered inbox notifications.",
		})
		return
	}

	unreadCount, err := api.Database.CountUnreadInboxNotificationsByUserID(ctx, apikey.UserID)
	if err != nil {
		api.Logger.Error(ctx, "failed to count unread inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to count unread inbox notifications.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ListInboxNotificationsResponse{
		Notifications: func() []codersdk.InboxNotification {
			notificationsList := make([]codersdk.InboxNotification, 0, len(notifs))
			for _, notification := range notifs {
				notificationsList = append(notificationsList, convertInboxNotificationResponse(ctx, api.Logger, notification))
			}
			return notificationsList
		}(),
		UnreadCount: int(unreadCount),
	})
}

// updateInboxNotificationReadStatus changes the read status of a notification.
// @Summary Update read status of a notification
// @ID update-read-status-of-a-notification
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Param id path string true "id of the notification"
// @Success 200 {object} codersdk.Response
// @Router /notifications/inbox/{id}/read-status [put]
func (api *API) updateInboxNotificationReadStatus(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)
	)

	notificationID, ok := httpmw.ParseUUIDParam(rw, r, "id")
	if !ok {

	}

	var body codersdk.UpdateInboxNotificationReadStatusRequest
	if !httpapi.Read(ctx, rw, r, &body) {
		return
	}

	err := api.Database.UpdateInboxNotificationReadStatus(ctx, database.UpdateInboxNotificationReadStatusParams{
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

	unreadCount, err := api.Database.CountUnreadInboxNotificationsByUserID(ctx, apikey.UserID)
	if err != nil {
		api.Logger.Error(ctx, "failed to call count unread inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to call count unread inbox notifications.",
		})
		return
	}

	updatedNotification, err := api.Database.GetInboxNotificationByID(ctx, notificationID)
	if err != nil {
		api.Logger.Error(ctx, "failed to get notification by id", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get notification by id.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UpdateInboxNotificationReadStatusResponse{
		Notification: convertInboxNotificationResponse(ctx, api.Logger, updatedNotification),
		UnreadCount:  int(unreadCount),
	})
}
