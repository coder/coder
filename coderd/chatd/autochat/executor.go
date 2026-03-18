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
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
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
	db     database.Store
	ps     pubsub.Pubsub
	log    slog.Logger
	createChat func(ctx context.Context, opts CreateChatOptions) (database.Chat, error)
}

// NewExecutor creates a new automation executor.
func NewExecutor(
	db database.Store,
	ps pubsub.Pubsub,
	createChatFn func(ctx context.Context, opts CreateChatOptions) (database.Chat, error),
	log slog.Logger,
) *Executor {
	return &Executor{
		db:         db,
		ps:         ps,
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

	// 4. Create chat in "pending" status for chatd to pick up.
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

	// 6. Watch for chat completion in the background and update
	// the run status accordingly.
	go e.watchRunCompletion(run.ID, chat.ID, automation.OwnerID)

	return run, nil
}

// watchRunCompletion subscribes to chat status change events and
// marks the automation run as completed or failed when the chat
// reaches a terminal state.
func (e *Executor) watchRunCompletion(
	runID uuid.UUID,
	chatID uuid.UUID,
	ownerID uuid.UUID,
) {
	ctx := context.Background()

	cancel, err := e.ps.SubscribeWithErr(
		coderdpubsub.ChatEventChannel(ownerID),
		coderdpubsub.HandleChatEvent(
			func(ctx context.Context, event coderdpubsub.ChatEvent, err error) {
				if err != nil {
					e.log.Error(ctx, "chat event subscription error",
						slog.F("run_id", runID),
						slog.Error(err),
					)
					return
				}

				// Only care about status changes for our chat.
				if event.Chat.ID != chatID {
					return
				}
				if event.Kind != coderdpubsub.ChatEventKindStatusChange {
					return
				}

				status := string(event.Chat.Status)
				switch status {
				case "completed", "error":
					// Terminal state reached.
				default:
					return
				}

				now := dbtime.Now()
				runStatus := "completed"
				var runError sql.NullString
				if status == "error" {
					runStatus = "failed"
					runError = sql.NullString{
						String: "chat ended with error status",
						Valid:  true,
					}
				}

				_, updateErr := e.db.UpdateChatAutomationRun(ctx,
					database.UpdateChatAutomationRunParams{
						ID:          runID,
						Status:      runStatus,
						Error:       runError,
						CompletedAt: sql.NullTime{Time: now, Valid: true},
					},
				)
				if updateErr != nil {
					e.log.Error(ctx, "failed to update completed run",
						slog.F("run_id", runID),
						slog.F("chat_id", chatID),
						slog.Error(updateErr),
					)
				} else {
					e.log.Info(ctx, "automation run completed",
						slog.F("run_id", runID),
						slog.F("chat_id", chatID),
						slog.F("status", runStatus),
					)
				}
			},
		),
	)
	if err != nil {
		e.log.Error(ctx, "failed to subscribe to chat events for run completion",
			slog.F("run_id", runID),
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return
	}

	// The subscription is cancelled implicitly when the pubsub is
	// closed during server shutdown. We store the cancel func to
	// be explicit, but in practice the server lifecycle manages it.
	_ = cancel
}
