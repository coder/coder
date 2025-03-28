package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	FallbackIconWorkspace = "DEFAULT_ICON_WORKSPACE"
	FallbackIconAccount   = "DEFAULT_ICON_ACCOUNT"
	FallbackIconTemplate  = "DEFAULT_ICON_TEMPLATE"
	FallbackIconOther     = "DEFAULT_ICON_OTHER"
)

type InboxNotification struct {
	ID         uuid.UUID                 `json:"id" format:"uuid"`
	UserID     uuid.UUID                 `json:"user_id" format:"uuid"`
	TemplateID uuid.UUID                 `json:"template_id" format:"uuid"`
	Targets    []uuid.UUID               `json:"targets" format:"uuid"`
	Title      string                    `json:"title"`
	Content    string                    `json:"content"`
	Icon       string                    `json:"icon"`
	Actions    []InboxNotificationAction `json:"actions"`
	ReadAt     *time.Time                `json:"read_at"`
	CreatedAt  time.Time                 `json:"created_at" format:"date-time"`
}

type InboxNotificationAction struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type GetInboxNotificationResponse struct {
	Notification InboxNotification `json:"notification"`
	UnreadCount  int               `json:"unread_count"`
}

type ListInboxNotificationsRequest struct {
	Targets        string `json:"targets,omitempty"`
	Templates      string `json:"templates,omitempty"`
	ReadStatus     string `json:"read_status,omitempty"`
	StartingBefore string `json:"starting_before,omitempty"`
}

type ListInboxNotificationsResponse struct {
	Notifications []InboxNotification `json:"notifications"`
	UnreadCount   int                 `json:"unread_count"`
}

func ListInboxNotificationsRequestToQueryParams(req ListInboxNotificationsRequest) []RequestOption {
	var opts []RequestOption
	if req.Targets != "" {
		opts = append(opts, WithQueryParam("targets", req.Targets))
	}
	if req.Templates != "" {
		opts = append(opts, WithQueryParam("templates", req.Templates))
	}
	if req.ReadStatus != "" {
		opts = append(opts, WithQueryParam("read_status", req.ReadStatus))
	}
	if req.StartingBefore != "" {
		opts = append(opts, WithQueryParam("starting_before", req.StartingBefore))
	}

	return opts
}

func (c *Client) ListInboxNotifications(ctx context.Context, req ListInboxNotificationsRequest) (ListInboxNotificationsResponse, error) {
	res, err := c.Request(
		ctx, http.MethodGet,
		"/api/v2/notifications/inbox",
		nil, ListInboxNotificationsRequestToQueryParams(req)...,
	)
	if err != nil {
		return ListInboxNotificationsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ListInboxNotificationsResponse{}, ReadBodyAsError(res)
	}

	var listInboxNotificationsResponse ListInboxNotificationsResponse
	return listInboxNotificationsResponse, json.NewDecoder(res.Body).Decode(&listInboxNotificationsResponse)
}

type UpdateInboxNotificationReadStatusRequest struct {
	IsRead bool `json:"is_read"`
}

type UpdateInboxNotificationReadStatusResponse struct {
	Notification InboxNotification `json:"notification"`
	UnreadCount  int               `json:"unread_count"`
}

func (c *Client) UpdateInboxNotificationReadStatus(ctx context.Context, notifID string, req UpdateInboxNotificationReadStatusRequest) (UpdateInboxNotificationReadStatusResponse, error) {
	res, err := c.Request(
		ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/notifications/inbox/%v/read-status", notifID),
		req,
	)
	if err != nil {
		return UpdateInboxNotificationReadStatusResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return UpdateInboxNotificationReadStatusResponse{}, ReadBodyAsError(res)
	}

	var resp UpdateInboxNotificationReadStatusResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) MarkAllInboxNotificationsAsRead(ctx context.Context) error {
	res, err := c.Request(
		ctx, http.MethodPut,
		"/api/v2/notifications/inbox/mark-all-as-read",
		nil,
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}

	return nil
}
