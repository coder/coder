package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type NotificationsSettings struct {
	NotifierPaused bool `json:"notifier_paused"`
}

type AlertTemplate struct {
	ID               uuid.UUID `json:"id" format:"uuid"`
	Name             string    `json:"name"`
	TitleTemplate    string    `json:"title_template"`
	BodyTemplate     string    `json:"body_template"`
	Actions          string    `json:"actions" format:""`
	Group            string    `json:"group"`
	Method           string    `json:"method"`
	Kind             string    `json:"kind"`
	EnabledByDefault bool      `json:"enabled_by_default"`
}

type AlertMethodsResponse struct {
	AvailableAlertMethods []string `json:"available"`
	DefaultAlertMethod    string   `json:"default"`
}

type AlertPreference struct {
	AlertTemplateID uuid.UUID `json:"id" format:"uuid"`
	Disabled        bool      `json:"disabled"`
	UpdatedAt       time.Time `json:"updated_at" format:"date-time"`
}

// GetNotificationsSettings retrieves the notifications settings, which currently just describes whether all
// notifications are paused from sending.
func (c *Client) GetNotificationsSettings(ctx context.Context) (NotificationsSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/notifications/settings", nil)
	if err != nil {
		return NotificationsSettings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return NotificationsSettings{}, ReadBodyAsError(res)
	}
	var settings NotificationsSettings
	return settings, json.NewDecoder(res.Body).Decode(&settings)
}

// PutNotificationsSettings modifies the notifications settings, which currently just controls whether all
// notifications are paused from sending.
func (c *Client) PutNotificationsSettings(ctx context.Context, settings NotificationsSettings) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/v2/notifications/settings", settings)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		return nil
	}
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// UpdateAlertTemplateMethod modifies a notification template to use a specific notification method, overriding
// the method set in the deployment configuration.
func (c *Client) UpdateAlertTemplateMethod(ctx context.Context, alertTemplateID uuid.UUID, method string) error {
	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/notifications/templates/%s/method", alertTemplateID),
		UpdateAlertTemplateMethod{Method: method},
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		return nil
	}
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// GetSystemAlertTemplates retrieves all notification templates pertaining to internal system events.
func (c *Client) GetSystemAlertTemplates(ctx context.Context) ([]AlertTemplate, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/notifications/templates/system", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var templates []AlertTemplate
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}

	if err := json.Unmarshal(body, &templates); err != nil {
		return nil, xerrors.Errorf("unmarshal response body: %w", err)
	}

	return templates, nil
}

// GetUserAlertPreferences retrieves notification preferences for a given user.
func (c *Client) GetUserAlertPreferences(ctx context.Context, userID uuid.UUID) ([]AlertPreference, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/notifications/preferences", userID.String()), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var prefs []AlertPreference
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}

	if err := json.Unmarshal(body, &prefs); err != nil {
		return nil, xerrors.Errorf("unmarshal response body: %w", err)
	}

	return prefs, nil
}

// UpdateUserAlertPreferences updates notification preferences for a given user.
func (c *Client) UpdateUserAlertPreferences(ctx context.Context, userID uuid.UUID, req UpdateUserAlertPreferences) ([]AlertPreference, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/notifications/preferences", userID.String()), req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var prefs []AlertPreference
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}

	if err := json.Unmarshal(body, &prefs); err != nil {
		return nil, xerrors.Errorf("unmarshal response body: %w", err)
	}

	return prefs, nil
}

// GetAlertDispatchMethods the available and default notification dispatch methods.
func (c *Client) GetAlertDispatchMethods(ctx context.Context) (AlertMethodsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/notifications/dispatch-methods", nil)
	if err != nil {
		return AlertMethodsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AlertMethodsResponse{}, ReadBodyAsError(res)
	}

	var resp AlertMethodsResponse
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return AlertMethodsResponse{}, xerrors.Errorf("read response body: %w", err)
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return AlertMethodsResponse{}, xerrors.Errorf("unmarshal response body: %w", err)
	}

	return resp, nil
}

func (c *Client) PostTestNotification(ctx context.Context) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/notifications/test", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type UpdateAlertTemplateMethod struct {
	Method string `json:"method,omitempty" example:"webhook"`
}

type UpdateUserAlertPreferences struct {
	TemplateDisabledMap map[string]bool `json:"template_disabled_map"`
}

type WebpushMessageAction struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type WebpushMessage struct {
	Icon    string                 `json:"icon"`
	Title   string                 `json:"title"`
	Body    string                 `json:"body"`
	Actions []WebpushMessageAction `json:"actions"`
}

type WebpushSubscription struct {
	Endpoint  string `json:"endpoint"`
	AuthKey   string `json:"auth_key"`
	P256DHKey string `json:"p256dh_key"`
}

type DeleteWebpushSubscription struct {
	Endpoint string `json:"endpoint"`
}

// PostWebpushSubscription creates a push notification subscription for a given user.
func (c *Client) PostWebpushSubscription(ctx context.Context, user string, req WebpushSubscription) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/webpush/subscription", user), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// DeleteWebpushSubscription deletes a push notification subscription for a given user.
// Think of this as an unsubscribe, but for a specific push notification subscription.
func (c *Client) DeleteWebpushSubscription(ctx context.Context, user string, req DeleteWebpushSubscription) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/users/%s/webpush/subscription", user), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) PostTestWebpushMessage(ctx context.Context) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/webpush/test", Me), WebpushMessage{
		Title: "It's working!",
		Body:  "You've subscribed to push notifications.",
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type CustomNotificationContent struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

type CustomNotificationRequest struct {
	Content *CustomNotificationContent `json:"content"`
	// TODO(ssncferreira): Add target (user_ids, roles) to support multi-user and role-based delivery.
	//   See: https://github.com/coder/coder/issues/19768
}

const (
	maxCustomNotificationTitleLen = 120
	maxCustomAlertMessageLen      = 2000
)

func (c CustomNotificationRequest) Validate() error {
	if c.Content == nil {
		return xerrors.Errorf("content is required")
	}
	return c.Content.Validate()
}

func (c CustomNotificationContent) Validate() error {
	if strings.TrimSpace(c.Title) == "" ||
		strings.TrimSpace(c.Message) == "" {
		return xerrors.Errorf("provide a non-empty 'content.title' and 'content.message'")
	}
	if len(c.Title) > maxCustomNotificationTitleLen {
		return xerrors.Errorf("'content.title' must be less than %d characters", maxCustomNotificationTitleLen)
	}
	if len(c.Message) > maxCustomAlertMessageLen {
		return xerrors.Errorf("'content.message' must be less than %d characters", maxCustomAlertMessageLen)
	}
	return nil
}

func (c *Client) PostCustomNotification(ctx context.Context, req CustomNotificationRequest) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/notifications/custom", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
