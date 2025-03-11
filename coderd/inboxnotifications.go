package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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
	ctx := r.Context()

	var (
		apikey          = httpmw.APIKey(r)
		targetsParam    = r.URL.Query().Get("targets")
		templatesParam  = r.URL.Query().Get("templates")
		readStatusParam = r.URL.Query().Get("read_status")
	)

	var targets []uuid.UUID
	if targetsParam != "" {
		splitTargets := strings.Split(targetsParam, ",")
		for _, target := range splitTargets {
			id, err := uuid.Parse(target)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid target ID.",
					Detail:  err.Error(),
				})
				return
			}
			targets = append(targets, id)
		}
	}

	var templates []uuid.UUID
	if templatesParam != "" {
		splitTemplates := strings.Split(templatesParam, ",")
		for _, template := range splitTemplates {
			id, err := uuid.Parse(template)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid template ID.",
					Detail:  err.Error(),
				})
				return
			}
			templates = append(templates, id)
		}
	}

	if readStatusParam != "" {
		readOptions := []string{
			string(database.InboxNotificationReadStatusRead),
			string(database.InboxNotificationReadStatusUnread),
			string(database.InboxNotificationReadStatusAll),
		}

		if !slices.Contains(readOptions, readStatusParam) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid read status.",
			})
			return
		}
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

	notificationCh := make(chan codersdk.InboxNotification, 1)

	closeInboxNotificationsSubscriber, err := api.Pubsub.SubscribeWithErr(pubsub.InboxNotificationForOwnerEventChannel(apikey.UserID),
		pubsub.HandleInboxNotificationEvent(
			func(ctx context.Context, payload pubsub.InboxNotificationEvent, err error) {
				if err != nil {
					api.Logger.Error(ctx, "inbox notification event", slog.Error(err))
					return
				}

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
				if readStatusParam != "" {
					if readStatusParam == string(database.InboxNotificationReadStatusRead) {
						if payload.InboxNotification.ReadAt == nil {
							return
						}
					} else if readStatusParam == string(database.InboxNotificationReadStatusUnread) {
						if payload.InboxNotification.ReadAt != nil {
							return
						}
					}
				}

				notificationCh <- payload.InboxNotification
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
				api.Logger.Error(ctx, "count unread inbox notifications", slog.Error(err))
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
	ctx := r.Context()

	var (
		apikey              = httpmw.APIKey(r)
		targetsParam        = r.URL.Query().Get("targets")
		templatesParam      = r.URL.Query().Get("templates")
		readStatusParam     = r.URL.Query().Get("read_status")
		startingBeforeParam = r.URL.Query().Get("starting_before")
	)

	var targets []uuid.UUID
	if targetsParam != "" {
		splitTargets := strings.Split(targetsParam, ",")
		for _, target := range splitTargets {
			id, err := uuid.Parse(target)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid target ID.",
					Detail:  err.Error(),
				})
				return
			}
			targets = append(targets, id)
		}
	}

	var templates []uuid.UUID
	if templatesParam != "" {
		splitTemplates := strings.Split(templatesParam, ",")
		for _, template := range splitTemplates {
			id, err := uuid.Parse(template)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid template ID.",
					Detail:  err.Error(),
				})
				return
			}
			templates = append(templates, id)
		}
	}

	readStatus := database.InboxNotificationReadStatusAll
	if readStatusParam != "" {
		readOptions := []string{
			string(database.InboxNotificationReadStatusRead),
			string(database.InboxNotificationReadStatusUnread),
			string(database.InboxNotificationReadStatusAll),
		}

		if !slices.Contains(readOptions, readStatusParam) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid read status.",
			})
			return
		}
		readStatus = database.InboxNotificationReadStatus(readStatusParam)
	}

	var startingBefore time.Time
	if startingBeforeParam != "" {
		lastNotifID, err := uuid.Parse(startingBeforeParam)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid starting before.",
			})
			return
		}
		lastNotif, err := api.Database.GetInboxNotificationByID(ctx, lastNotifID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid starting before.",
			})
			return
		}
		startingBefore = lastNotif.CreatedAt
	}

	notifications, err := api.Database.GetFilteredInboxNotificationsByUserID(ctx, database.GetFilteredInboxNotificationsByUserIDParams{
		UserID:       apikey.UserID,
		Templates:    templates,
		Targets:      targets,
		ReadStatus:   readStatus,
		CreatedAtOpt: startingBefore,
	})
	if err != nil {
		api.Logger.Error(ctx, "get filtered inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get inbox notifications.",
		})
		return
	}

	unreadCount, err := api.Database.CountUnreadInboxNotificationsByUserID(ctx, apikey.UserID)
	if err != nil {
		api.Logger.Error(ctx, "count unread inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to count unread inbox notifications.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ListInboxNotificationsResponse{
		Notifications: func() []codersdk.InboxNotification {
			var notificationsList []codersdk.InboxNotification
			for _, notification := range notifications {
				notificationsList = append(notificationsList, codersdk.InboxNotification{
					ID:         notification.ID,
					UserID:     notification.UserID,
					TemplateID: notification.TemplateID,
					Targets:    notification.Targets,
					Title:      notification.Title,
					Content:    notification.Content,
					Icon:       notification.Icon,
					Actions: func() []codersdk.InboxNotificationAction {
						var actionsList []codersdk.InboxNotificationAction
						err := json.Unmarshal([]byte(notification.Actions), &actionsList)
						if err != nil {
							api.Logger.Error(ctx, "unmarshal inbox notification actions", slog.Error(err))
						}
						return actionsList
					}(),
					ReadAt: func() *time.Time {
						if !notification.ReadAt.Valid {
							return nil
						}
						return &notification.ReadAt.Time
					}(),
					CreatedAt: notification.CreatedAt,
				})
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
// @Success 201 {object} codersdk.Response
// @Router /notifications/inbox/{id}/read-status [put]
func (api *API) updateInboxNotificationReadStatus(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var (
		apikey  = httpmw.APIKey(r)
		notifID = chi.URLParam(r, "id")
	)

	var body codersdk.UpdateInboxNotificationReadStatusRequest
	if !httpapi.Read(ctx, rw, r, &body) {
		return
	}

	parsedNotifID, err := uuid.Parse(notifID)
	if err != nil {
		api.Logger.Error(ctx, "failed to parse uuid", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to parse notification uuid.",
		})
		return
	}

	err = api.Database.UpdateInboxNotificationReadStatus(ctx, database.UpdateInboxNotificationReadStatusParams{
		ID: parsedNotifID,
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
		api.Logger.Error(ctx, "get filtered inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get inbox notifications.",
		})
		return
	}

	unreadCount, err := api.Database.CountUnreadInboxNotificationsByUserID(ctx, apikey.UserID)
	if err != nil {
		api.Logger.Error(ctx, "count unread inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to count unread inbox notifications.",
		})
		return
	}

	updatedNotification, err := api.Database.GetInboxNotificationByID(ctx, parsedNotifID)
	if err != nil {
		api.Logger.Error(ctx, "count unread inbox notifications", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to count unread inbox notifications.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UpdateInboxNotificationReadStatusResponse{
		Notification: codersdk.InboxNotification{
			ID:         updatedNotification.ID,
			UserID:     updatedNotification.UserID,
			TemplateID: updatedNotification.TemplateID,
			Targets:    updatedNotification.Targets,
			Title:      updatedNotification.Title,
			Content:    updatedNotification.Content,
			Icon:       updatedNotification.Icon,
			Actions: func() []codersdk.InboxNotificationAction {
				var actionsList []codersdk.InboxNotificationAction
				err := json.Unmarshal([]byte(updatedNotification.Actions), &actionsList)
				if err != nil {
					api.Logger.Error(ctx, "unmarshal inbox notification actions", slog.Error(err))
				}
				return actionsList
			}(),
			ReadAt: func() *time.Time {
				if !updatedNotification.ReadAt.Valid {
					return nil
				}
				return &updatedNotification.ReadAt.Time
			}(),
			CreatedAt: updatedNotification.CreatedAt,
		},
		UnreadCount: int(unreadCount),
	})
}
