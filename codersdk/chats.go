package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Chat represents a persisted chat transcript owned by a user.
type Chat struct {
	ID             uuid.UUID       `json:"id" format:"uuid"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	OrganizationID uuid.UUID       `json:"organization_id" format:"uuid"`
	OwnerID        uuid.UUID       `json:"owner_id" format:"uuid"`
	WorkspaceID    uuid.NullUUID   `json:"workspace_id" format:"uuid"`
	Title          string          `json:"title,omitempty"`
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	Metadata       json.RawMessage `json:"metadata"`
}

// ChatMessage is an append-only row in chat_messages.
//
// Content is a JSON envelope. The role determines how to interpret the envelope.
type ChatMessage struct {
	ChatID    uuid.UUID       `json:"chat_id" format:"uuid"`
	ID        int64           `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
}

type CreateChatRequest struct {
	WorkspaceID *uuid.UUID      `json:"workspace_id,omitempty" format:"uuid"`
	Title       *string         `json:"title,omitempty"`
	Provider    string          `json:"provider"`
	Model       string          `json:"model"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type CreateChatMessageRequest struct {
	Content string `json:"content"`
}

type CreateChatMessageResponse struct {
	RunID   string      `json:"run_id"`
	Message ChatMessage `json:"message"`
}

func (c *Client) CreateChat(ctx context.Context, req CreateChatRequest) (Chat, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/chats", req)
	if err != nil {
		return Chat{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Chat{}, ReadBodyAsError(res)
	}
	var out Chat
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return Chat{}, err
	}
	return out, nil
}

func (c *Client) Chat(ctx context.Context, chatID uuid.UUID) (Chat, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s", chatID), nil)
	if err != nil {
		return Chat{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Chat{}, ReadBodyAsError(res)
	}
	var out Chat
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return Chat{}, err
	}
	return out, nil
}

func (c *Client) ChatMessages(ctx context.Context, chatID uuid.UUID) ([]ChatMessage, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/messages", chatID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var out []ChatMessage
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateChatMessage(ctx context.Context, chatID uuid.UUID, req CreateChatMessageRequest) (CreateChatMessageResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/%s/messages", chatID), req)
	if err != nil {
		return CreateChatMessageResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return CreateChatMessageResponse{}, ReadBodyAsError(res)
	}
	var out CreateChatMessageResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return CreateChatMessageResponse{}, err
	}
	return out, nil
}
