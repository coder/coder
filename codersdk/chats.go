package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ChatStatus represents the status of a chat.
type ChatStatus string

const (
	ChatStatusWaiting   ChatStatus = "waiting"
	ChatStatusPending   ChatStatus = "pending"
	ChatStatusRunning   ChatStatus = "running"
	ChatStatusPaused    ChatStatus = "paused"
	ChatStatusCompleted ChatStatus = "completed"
	ChatStatusError     ChatStatus = "error"
)

// Chat represents a chat session with an AI agent.
type Chat struct {
	ID               uuid.UUID       `json:"id" format:"uuid"`
	OwnerID          uuid.UUID       `json:"owner_id" format:"uuid"`
	WorkspaceID      *uuid.UUID      `json:"workspace_id,omitempty" format:"uuid"`
	WorkspaceAgentID *uuid.UUID      `json:"workspace_agent_id,omitempty" format:"uuid"`
	Title            string          `json:"title"`
	Status           ChatStatus      `json:"status"`
	ModelConfig      json.RawMessage `json:"model_config,omitempty"`
	CreatedAt        time.Time       `json:"created_at" format:"date-time"`
	UpdatedAt        time.Time       `json:"updated_at" format:"date-time"`
}

// ChatMessage represents a single message in a chat.
type ChatMessage struct {
	ID         int64           `json:"id"`
	ChatID     uuid.UUID       `json:"chat_id" format:"uuid"`
	CreatedAt  time.Time       `json:"created_at" format:"date-time"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID *string         `json:"tool_call_id,omitempty"`
	Thinking   *string         `json:"thinking,omitempty"`
	Hidden     bool            `json:"hidden"`
}

// CreateChatRequest is the request to create a new chat.
type CreateChatRequest struct {
	Title            string          `json:"title"`
	WorkspaceID      *uuid.UUID      `json:"workspace_id,omitempty" format:"uuid"`
	WorkspaceAgentID *uuid.UUID      `json:"workspace_agent_id,omitempty" format:"uuid"`
	ModelConfig      json.RawMessage `json:"model_config,omitempty"`
}

// UpdateChatRequest is the request to update a chat.
type UpdateChatRequest struct {
	Title string `json:"title"`
}

// CreateChatMessageRequest is the request to add a message to a chat.
type CreateChatMessageRequest struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID *string         `json:"tool_call_id,omitempty"`
	Thinking   *string         `json:"thinking,omitempty"`
}

// ChatWithMessages is a chat along with its messages.
type ChatWithMessages struct {
	Chat     Chat          `json:"chat"`
	Messages []ChatMessage `json:"messages"`
}

// ChatGitChange represents a git file change detected during a chat session.
type ChatGitChange struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	ChatID      uuid.UUID `json:"chat_id" format:"uuid"`
	FilePath    string    `json:"file_path"`
	ChangeType  string    `json:"change_type"` // added, modified, deleted, renamed
	OldPath     *string   `json:"old_path,omitempty"`
	DiffSummary *string   `json:"diff_summary,omitempty"`
	DetectedAt  time.Time `json:"detected_at" format:"date-time"`
}

// ListChats returns all chats for the authenticated user.
func (c *Client) ListChats(ctx context.Context) ([]Chat, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/chats", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var chats []Chat
	return chats, json.NewDecoder(res.Body).Decode(&chats)
}

// CreateChat creates a new chat.
func (c *Client) CreateChat(ctx context.Context, req CreateChatRequest) (Chat, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/chats", req)
	if err != nil {
		return Chat{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Chat{}, ReadBodyAsError(res)
	}
	var chat Chat
	return chat, json.NewDecoder(res.Body).Decode(&chat)
}

// GetChat returns a chat by ID, including its messages.
func (c *Client) GetChat(ctx context.Context, chatID uuid.UUID) (ChatWithMessages, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s", chatID), nil)
	if err != nil {
		return ChatWithMessages{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatWithMessages{}, ReadBodyAsError(res)
	}
	var chat ChatWithMessages
	return chat, json.NewDecoder(res.Body).Decode(&chat)
}

// DeleteChat deletes a chat by ID.
func (c *Client) DeleteChat(ctx context.Context, chatID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/chats/%s", chatID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// CreateChatMessage adds a message to a chat.
func (c *Client) CreateChatMessage(ctx context.Context, chatID uuid.UUID, req CreateChatMessageRequest) ([]ChatMessage, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/%s/messages", chatID), req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var messages []ChatMessage
	return messages, json.NewDecoder(res.Body).Decode(&messages)
}

// GetChatGitChanges returns git changes for a chat.
func (c *Client) GetChatGitChanges(ctx context.Context, chatID uuid.UUID) ([]ChatGitChange, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/git-changes", chatID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var changes []ChatGitChange
	return changes, json.NewDecoder(res.Body).Decode(&changes)
}
