package automations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
)

// ChatCreator abstracts the chatd.Server so the automations package
// does not depend on it directly. Both the webhook handler and the
// cron scheduler inject the real implementation.
type ChatCreator interface {
	// CreateChat creates a new chat and sends the initial message.
	// The returned UUID is the chat ID.
	CreateChat(ctx context.Context, opts CreateChatOptions) (uuid.UUID, error)
	// SendMessage appends a user message to an existing chat.
	SendMessage(ctx context.Context, chatID uuid.UUID, ownerID uuid.UUID, content string) error
}

// CreateChatOptions contains everything needed to create an
// automation-initiated chat.
type CreateChatOptions struct {
	OwnerID       uuid.UUID
	AutomationID  uuid.UUID
	Title         string
	Instructions  string
	ModelConfigID uuid.NullUUID
	MCPServerIDs  []uuid.UUID
	Labels        map[string]string
}

// FireResult is the outcome of an automation trigger firing.
type FireResult struct {
	Status        string
	MatchedChatID uuid.NullUUID
	CreatedChatID uuid.NullUUID
	Error         string
}

// FireOptions contains the inputs for firing an automation trigger.
type FireOptions struct {
	// Automation fields.
	AutomationID            uuid.UUID
	AutomationName          string
	AutomationStatus        string
	AutomationOwnerID       uuid.UUID
	AutomationInstructions  string
	AutomationModelConfigID uuid.NullUUID
	AutomationMCPServerIDs  []uuid.UUID
	AutomationAllowedTools  []string
	MaxChatCreatesPerHour   int32
	MaxMessagesPerHour      int32

	// Trigger fields.
	TriggerID uuid.UUID

	// Resolved data.
	Payload        json.RawMessage
	FilterMatched  bool
	ResolvedLabels map[string]string
}

// Fire executes the active-mode logic for an automation trigger:
// rate-limit check, find-or-create chat, send message, and record
// the event. For preview mode it only logs the event without
// creating a chat.
func Fire(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	chat ChatCreator,
	opts FireOptions,
) FireResult {
	triggerUUID := uuid.NullUUID{UUID: opts.TriggerID, Valid: true}

	// Resolve labels JSON for the event record.
	var resolvedLabelsJSON pqtype.NullRawMessage
	if len(opts.ResolvedLabels) > 0 {
		if j, err := json.Marshal(opts.ResolvedLabels); err == nil {
			resolvedLabelsJSON = pqtype.NullRawMessage{RawMessage: j, Valid: true}
		}
	}

	// Preview mode: log the event, optionally look up a matching
	// chat, but never create or continue one.
	if opts.AutomationStatus == "preview" {
		result := FireResult{Status: "preview"}
		// Try to find a matching chat for the preview log.
		if len(opts.ResolvedLabels) > 0 {
			if chatID, ok := findChatByLabels(ctx, db, opts.AutomationOwnerID, opts.ResolvedLabels); ok {
				result.MatchedChatID = uuid.NullUUID{UUID: chatID, Valid: true}
			}
		}
		insertEvent(ctx, db, database.InsertAutomationEventParams{
			AutomationID:   opts.AutomationID,
			TriggerID:      triggerUUID,
			Payload:        opts.Payload,
			FilterMatched:  opts.FilterMatched,
			ResolvedLabels: resolvedLabelsJSON,
			MatchedChatID:  result.MatchedChatID,
			Status:         "preview",
		})
		return result
	}

	// Active mode: enforce rate limits.
	windowStart := time.Now().Add(-time.Hour)

	chatCreates, err := db.CountAutomationChatCreatesInWindow(ctx, database.CountAutomationChatCreatesInWindowParams{
		AutomationID: opts.AutomationID,
		WindowStart:  windowStart,
	})
	if err != nil {
		logger.Error(ctx, "failed to count chat creates", slog.Error(err))
		insertEvent(ctx, db, database.InsertAutomationEventParams{
			AutomationID:   opts.AutomationID,
			TriggerID:      triggerUUID,
			Payload:        opts.Payload,
			FilterMatched:  opts.FilterMatched,
			ResolvedLabels: resolvedLabelsJSON,
			Status:         "error",
			Error:          sql.NullString{String: "failed to check rate limits", Valid: true},
		})
		return FireResult{Status: "error", Error: "failed to check rate limits"}
	}

	msgCount, err := db.CountAutomationMessagesInWindow(ctx, database.CountAutomationMessagesInWindowParams{
		AutomationID: opts.AutomationID,
		WindowStart:  windowStart,
	})
	if err != nil {
		logger.Error(ctx, "failed to count messages", slog.Error(err))
		insertEvent(ctx, db, database.InsertAutomationEventParams{
			AutomationID:   opts.AutomationID,
			TriggerID:      triggerUUID,
			Payload:        opts.Payload,
			FilterMatched:  opts.FilterMatched,
			ResolvedLabels: resolvedLabelsJSON,
			Status:         "error",
			Error:          sql.NullString{String: "failed to check rate limits", Valid: true},
		})
		return FireResult{Status: "error", Error: "failed to check rate limits"}
	}

	// Check message rate limit (applies to both create and continue).
	if msgCount >= int64(opts.MaxMessagesPerHour) {
		insertEvent(ctx, db, database.InsertAutomationEventParams{
			AutomationID:   opts.AutomationID,
			TriggerID:      triggerUUID,
			Payload:        opts.Payload,
			FilterMatched:  opts.FilterMatched,
			ResolvedLabels: resolvedLabelsJSON,
			Status:         "rate_limited",
			Error: sql.NullString{
				String: fmt.Sprintf("message rate limit exceeded: %d/%d per hour", msgCount, opts.MaxMessagesPerHour),
				Valid:  true,
			},
		})
		return FireResult{Status: "rate_limited", Error: "message rate limit exceeded"}
	}

	// Try to find an existing chat with matching labels to continue.
	if len(opts.ResolvedLabels) > 0 {
		if chatID, ok := findChatByLabels(ctx, db, opts.AutomationOwnerID, opts.ResolvedLabels); ok {
			// Continue existing chat.
			if err := chat.SendMessage(ctx, chatID, opts.AutomationOwnerID, opts.AutomationInstructions); err != nil {
				logger.Error(ctx, "failed to send message to existing chat",
					slog.F("chat_id", chatID),
					slog.Error(err),
				)
				insertEvent(ctx, db, database.InsertAutomationEventParams{
					AutomationID:   opts.AutomationID,
					TriggerID:      triggerUUID,
					Payload:        opts.Payload,
					FilterMatched:  opts.FilterMatched,
					ResolvedLabels: resolvedLabelsJSON,
					MatchedChatID:  uuid.NullUUID{UUID: chatID, Valid: true},
					Status:         "error",
					Error:          sql.NullString{String: "failed to send message to chat", Valid: true},
				})
				return FireResult{Status: "error", Error: "failed to send message"}
			}
			insertEvent(ctx, db, database.InsertAutomationEventParams{
				AutomationID:   opts.AutomationID,
				TriggerID:      triggerUUID,
				Payload:        opts.Payload,
				FilterMatched:  opts.FilterMatched,
				ResolvedLabels: resolvedLabelsJSON,
				MatchedChatID:  uuid.NullUUID{UUID: chatID, Valid: true},
				Status:         "continued",
			})
			return FireResult{
				Status:        "continued",
				MatchedChatID: uuid.NullUUID{UUID: chatID, Valid: true},
			}
		}
	}

	// No matching chat found — create a new one.
	// Check chat creation rate limit.
	if chatCreates >= int64(opts.MaxChatCreatesPerHour) {
		insertEvent(ctx, db, database.InsertAutomationEventParams{
			AutomationID:   opts.AutomationID,
			TriggerID:      triggerUUID,
			Payload:        opts.Payload,
			FilterMatched:  opts.FilterMatched,
			ResolvedLabels: resolvedLabelsJSON,
			Status:         "rate_limited",
			Error: sql.NullString{
				String: fmt.Sprintf("chat creation rate limit exceeded: %d/%d per hour", chatCreates, opts.MaxChatCreatesPerHour),
				Valid:  true,
			},
		})
		return FireResult{Status: "rate_limited", Error: "chat creation rate limit exceeded"}
	}

	newChatID, err := chat.CreateChat(ctx, CreateChatOptions{
		OwnerID:       opts.AutomationOwnerID,
		AutomationID:  opts.AutomationID,
		Title:         fmt.Sprintf("[%s] %s", opts.AutomationName, time.Now().UTC().Format("2006-01-02 15:04")),
		Instructions:  opts.AutomationInstructions,
		ModelConfigID: opts.AutomationModelConfigID,
		MCPServerIDs:  opts.AutomationMCPServerIDs,
		Labels:        opts.ResolvedLabels,
	})
	if err != nil {
		logger.Error(ctx, "failed to create chat", slog.Error(err))
		insertEvent(ctx, db, database.InsertAutomationEventParams{
			AutomationID:   opts.AutomationID,
			TriggerID:      triggerUUID,
			Payload:        opts.Payload,
			FilterMatched:  opts.FilterMatched,
			ResolvedLabels: resolvedLabelsJSON,
			Status:         "error",
			Error:          sql.NullString{String: "failed to create chat", Valid: true},
		})
		return FireResult{Status: "error", Error: "failed to create chat"}
	}

	insertEvent(ctx, db, database.InsertAutomationEventParams{
		AutomationID:   opts.AutomationID,
		TriggerID:      triggerUUID,
		Payload:        opts.Payload,
		FilterMatched:  opts.FilterMatched,
		ResolvedLabels: resolvedLabelsJSON,
		CreatedChatID:  uuid.NullUUID{UUID: newChatID, Valid: true},
		Status:         "created",
	})
	return FireResult{
		Status:        "created",
		CreatedChatID: uuid.NullUUID{UUID: newChatID, Valid: true},
	}
}

// findChatByLabels looks up an existing chat owned by the given user
// whose labels match the resolved label set.
func findChatByLabels(ctx context.Context, db database.Store, ownerID uuid.UUID, labels map[string]string) (uuid.UUID, bool) {
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return uuid.Nil, false
	}
	chats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnerID: ownerID,
		LabelFilter: pqtype.NullRawMessage{
			RawMessage: labelsJSON,
			Valid:      true,
		},
		LimitOpt: 1,
	})
	if err != nil || len(chats) == 0 {
		return uuid.Nil, false
	}
	return chats[0].ID, true
}

// insertEvent is a fire-and-forget helper that logs errors but never
// fails the caller.
func insertEvent(ctx context.Context, db database.Store, params database.InsertAutomationEventParams) {
	_, _ = db.InsertAutomationEvent(ctx, params)
}
