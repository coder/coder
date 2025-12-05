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
	InboxAlertFallbackIconWorkspace = "DEFAULT_ICON_WORKSPACE"
	InboxAlertFallbackIconAccount   = "DEFAULT_ICON_ACCOUNT"
	InboxAlertFallbackIconTemplate  = "DEFAULT_ICON_TEMPLATE"
	InboxAlertFallbackIconOther     = "DEFAULT_ICON_OTHER"
)

type InboxAlert struct {
	ID         uuid.UUID          `json:"id" format:"uuid"`
	UserID     uuid.UUID          `json:"user_id" format:"uuid"`
	TemplateID uuid.UUID          `json:"template_id" format:"uuid"`
	Targets    []uuid.UUID        `json:"targets" format:"uuid"`
	Title      string             `json:"title"`
	Content    string             `json:"content"`
	Icon       string             `json:"icon"`
	Actions    []InboxAlertAction `json:"actions"`
	ReadAt     *time.Time         `json:"read_at"`
	CreatedAt  time.Time          `json:"created_at" format:"date-time"`
}

type InboxAlertAction struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type GetInboxAlertResponse struct {
	Notification InboxAlert `json:"notification"`
	UnreadCount  int        `json:"unread_count"`
}

type ListInboxAlertsRequest struct {
	Targets        string `json:"targets,omitempty"`
	Templates      string `json:"templates,omitempty"`
	ReadStatus     string `json:"read_status,omitempty"`
	StartingBefore string `json:"starting_before,omitempty"`
}

type ListInboxAlertsResponse struct {
	Notifications []InboxAlert `json:"notifications"`
	UnreadCount   int          `json:"unread_count"`
}

func ListInboxAlertsRequestToQueryParams(req ListInboxAlertsRequest) []RequestOption {
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

func (c *Client) ListInboxAlerts(ctx context.Context, req ListInboxAlertsRequest) (ListInboxAlertsResponse, error) {
	res, err := c.Request(
		ctx, http.MethodGet,
		"/api/v2/notifications/inbox",
		nil, ListInboxAlertsRequestToQueryParams(req)...,
	)
	if err != nil {
		return ListInboxAlertsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ListInboxAlertsResponse{}, ReadBodyAsError(res)
	}

	var listInboxAlertsResponse ListInboxAlertsResponse
	return listInboxAlertsResponse, json.NewDecoder(res.Body).Decode(&listInboxAlertsResponse)
}

type UpdateInboxAlertReadStatusRequest struct {
	IsRead bool `json:"is_read"`
}

type UpdateInboxAlertReadStatusResponse struct {
	Notification InboxAlert `json:"notification"`
	UnreadCount  int        `json:"unread_count"`
}

func (c *Client) UpdateInboxAlertReadStatus(ctx context.Context, notifID string, req UpdateInboxAlertReadStatusRequest) (UpdateInboxAlertReadStatusResponse, error) {
	res, err := c.Request(
		ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/notifications/inbox/%v/read-status", notifID),
		req,
	)
	if err != nil {
		return UpdateInboxAlertReadStatusResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return UpdateInboxAlertReadStatusResponse{}, ReadBodyAsError(res)
	}

	var resp UpdateInboxAlertReadStatusResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) MarkAllInboxAlertsAsRead(ctx context.Context) error {
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
