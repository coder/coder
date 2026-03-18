package autochat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

// CreateChatOptions holds the parameters for creating an automated chat.
type CreateChatOptions struct {
	OwnerID            uuid.UUID
	AutomationID       *uuid.UUID
	Title              string
	ModelConfigID      uuid.UUID
	SystemPrompt       string
	InitialUserContent []codersdk.ChatMessagePart
}

// Executor handles firing automations by creating chats.
type Executor struct {
	db  database.Store
	log slog.Logger
	// createChat is a function that creates a chat. In production this
	// calls chatd.Server.CreateChat. It's a function field for
	// testability.
	createChat func(ctx context.Context, opts CreateChatOptions) (database.Chat, error)
}

// NewExecutor creates a new automation executor.
func NewExecutor(
	db database.Store,
	createChatFn func(ctx context.Context, opts CreateChatOptions) (database.Chat, error),
	log slog.Logger,
) *Executor {
	return &Executor{
		db:         db,
		createChat: createChatFn,
		log:        log.Named("autochat"),
	}
}

// Fire triggers an automation: checks concurrency, renders the prompt,
// creates a chat, and records the run.
func (e *Executor) Fire(
	ctx context.Context,
	automation database.ChatAutomation,
	triggerPayload json.RawMessage,
	templateData map[string]any,
) (database.ChatAutomationRun, error) {
	// 1. Check concurrency limit.
	active, err := e.db.CountActiveChatAutomationRuns(ctx, automation.ID)
	if err != nil {
		return database.ChatAutomationRun{}, xerrors.Errorf("count active runs: %w", err)
	}
	if active >= int64(automation.MaxConcurrentRuns) {
		return database.ChatAutomationRun{}, xerrors.Errorf(
			"automation %q has %d active runs (max %d)",
			automation.Name, active, automation.MaxConcurrentRuns,
		)
	}

	// 2. Render prompt template.
	rendered, err := RenderPrompt(automation.PromptTemplate, templateData)
	if err != nil {
		return database.ChatAutomationRun{}, xerrors.Errorf("render prompt: %w", err)
	}

	// 3. Insert run record.
	if triggerPayload == nil {
		triggerPayload = json.RawMessage("{}")
	}
	run, err := e.db.InsertChatAutomationRun(ctx, database.InsertChatAutomationRunParams{
		AutomationID:   automation.ID,
		TriggerPayload: triggerPayload,
		RenderedPrompt: rendered,
	})
	if err != nil {
		return database.ChatAutomationRun{}, xerrors.Errorf("insert run: %w", err)
	}

	// 4. Create chat - enters 'pending', chatd picks it up.
	chat, err := e.createChat(ctx, CreateChatOptions{
		OwnerID:       automation.OwnerID,
		AutomationID:  &automation.ID,
		Title:         fmt.Sprintf("[auto] %s", automation.Name),
		ModelConfigID: automation.ModelConfigID,
		SystemPrompt:  automation.SystemPrompt,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(rendered),
		},
	})
	if err != nil {
		// Mark run as failed.
		_, dbErr := e.db.UpdateChatAutomationRun(ctx, database.UpdateChatAutomationRunParams{
			ID:     run.ID,
			Status: "failed",
			Error:  sql.NullString{String: err.Error(), Valid: true},
		})
		if dbErr != nil {
			e.log.Error(ctx, "failed to mark automation run as failed",
				slog.F("run_id", run.ID),
				slog.Error(dbErr),
			)
		}
		return run, xerrors.Errorf("create chat: %w", err)
	}

	// 5. Link run to chat, mark running.
	now := dbtime.Now()
	run, err = e.db.UpdateChatAutomationRun(ctx, database.UpdateChatAutomationRunParams{
		ID:        run.ID,
		ChatID:    uuid.NullUUID{UUID: chat.ID, Valid: true},
		Status:    "running",
		StartedAt: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return run, xerrors.Errorf("update run: %w", err)
	}

	e.log.Info(ctx, "automation fired",
		slog.F("automation_id", automation.ID),
		slog.F("automation_name", automation.Name),
		slog.F("run_id", run.ID),
		slog.F("chat_id", chat.ID),
	)

	return run, nil
}
