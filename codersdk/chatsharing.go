package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ShareableChatOwners controls whose chats can be shared within an organization.
type ShareableChatOwners string

const (
	ShareableChatOwnersNone            ShareableChatOwners = "none"
	ShareableChatOwnersEveryone        ShareableChatOwners = "everyone"
	ShareableChatOwnersServiceAccounts ShareableChatOwners = "service_accounts"
)

// ChatSharingSettings represents chat sharing settings affecting an organization.
type ChatSharingSettings struct {
	SharingGloballyDisabled bool                `json:"sharing_globally_disabled"`
	ShareableChatOwners     ShareableChatOwners `json:"shareable_chat_owners" enums:"none,everyone,service_accounts"`
}

// UpdateChatSharingSettingsRequest represents chat sharing settings that can be updated for an organization.
type UpdateChatSharingSettingsRequest struct {
	ShareableChatOwners ShareableChatOwners `json:"shareable_chat_owners,omitempty" enums:"none,everyone,service_accounts"`
}

// ChatSharingSettings retrieves the chat sharing settings for an organization.
func (c *Client) ChatSharingSettings(ctx context.Context, orgID string) (ChatSharingSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/settings/chat-sharing", orgID), nil)
	if err != nil {
		return ChatSharingSettings{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatSharingSettings{}, ReadBodyAsError(res)
	}
	var resp ChatSharingSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// PatchChatSharingSettings modifies the chat sharing settings for an organization.
func (c *Client) PatchChatSharingSettings(ctx context.Context, orgID string, req UpdateChatSharingSettingsRequest) (ChatSharingSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/chat-sharing", orgID), req)
	if err != nil {
		return ChatSharingSettings{}, err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatSharingSettings{}, ReadBodyAsError(res)
	}
	var resp ChatSharingSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
