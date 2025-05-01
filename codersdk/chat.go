package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kylecarbs/aisdk-go"
	"golang.org/x/xerrors"
)

// CreateChat creates a new chat.
func (c *Client) CreateChat(ctx context.Context) (Chat, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/chats", nil)
	if err != nil {
		return Chat{}, xerrors.Errorf("execute request: %w", err)
	}
	if res.StatusCode != http.StatusCreated {
		return Chat{}, ReadBodyAsError(res)
	}
	defer res.Body.Close()
	var chat Chat
	return chat, json.NewDecoder(res.Body).Decode(&chat)
}

type Chat struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	UpdatedAt time.Time `json:"updated_at" format:"date-time"`
	Title     string    `json:"title"`
}

// ListChats lists all chats.
func (c *Client) ListChats(ctx context.Context) ([]Chat, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/chats", nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var chats []Chat
	return chats, json.NewDecoder(res.Body).Decode(&chats)
}

// Chat returns a chat by ID.
func (c *Client) Chat(ctx context.Context, id uuid.UUID) (Chat, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s", id), nil)
	if err != nil {
		return Chat{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Chat{}, ReadBodyAsError(res)
	}
	var chat Chat
	return chat, json.NewDecoder(res.Body).Decode(&chat)
}

// ChatMessages returns the messages of a chat.
func (c *Client) ChatMessages(ctx context.Context, id uuid.UUID) ([]ChatMessage, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/messages", id), nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var messages []ChatMessage
	return messages, json.NewDecoder(res.Body).Decode(&messages)
}

type ChatMessage = aisdk.Message

type CreateChatMessageRequest struct {
	Model    string      `json:"model"`
	Message  ChatMessage `json:"message"`
	Thinking bool        `json:"thinking"`
}

// CreateChatMessage creates a new chat message and streams the response.
// If the provided message has a conflicting ID with an existing message,
// it will be overwritten.
func (c *Client) CreateChatMessage(ctx context.Context, id uuid.UUID, req CreateChatMessageRequest) (<-chan aisdk.DataStreamPart, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/%s/messages", id), req)
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	nextEvent := ServerSentEventReader(ctx, res.Body)

	wc := make(chan aisdk.DataStreamPart, 256)
	go func() {
		defer close(wc)
		defer res.Body.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				sse, err := nextEvent()
				if err != nil {
					return
				}
				if sse.Type != ServerSentEventTypeData {
					continue
				}
				var part aisdk.DataStreamPart
				b, ok := sse.Data.([]byte)
				if !ok {
					return
				}
				err = json.Unmarshal(b, &part)
				if err != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case wc <- part:
				}
			}
		}
	}()

	return wc, nil
}

func (c *Client) DeleteChat(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/chats/%s", id), nil)
	if err != nil {
		return xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
