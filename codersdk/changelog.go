package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
)

type ChangelogEntry struct {
	Version  string `json:"version"`
	Title    string `json:"title"`
	Date     string `json:"date"`
	Summary  string `json:"summary"`
	ImageURL string `json:"image_url"`
	Content  string `json:"content,omitempty"`
}

type ListChangelogEntriesResponse struct {
	Entries []ChangelogEntry `json:"entries"`
}

type UnreadChangelogNotificationResponse struct {
	Notification *InboxNotification `json:"notification"`
}

func (c *Client) ListChangelogEntries(ctx context.Context) (ListChangelogEntriesResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/changelog", nil)
	if err != nil {
		return ListChangelogEntriesResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ListChangelogEntriesResponse{}, ReadBodyAsError(res)
	}

	var resp ListChangelogEntriesResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) GetChangelogEntry(ctx context.Context, version string) (ChangelogEntry, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/changelog/"+version, nil)
	if err != nil {
		return ChangelogEntry{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChangelogEntry{}, ReadBodyAsError(res)
	}

	var resp ChangelogEntry
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) UnreadChangelogNotification(ctx context.Context) (UnreadChangelogNotificationResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/changelog/unread", nil)
	if err != nil {
		return UnreadChangelogNotificationResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return UnreadChangelogNotificationResponse{}, ReadBodyAsError(res)
	}

	var resp UnreadChangelogNotificationResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
