package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

var ErrSubagentNotDescendant = xerrors.New("target chat is not a descendant of current chat")

const (
	subagentAwaitPollInterval  = 200 * time.Millisecond
	subagentAwaitFallbackPoll  = 5 * time.Second
	defaultSubagentWaitTimeout = 5 * time.Minute
)

// computerUseSubagentSystemPrompt is the system prompt prepended to
// every computer use subagent chat. It instructs the model on how to
// interact with the desktop environment via the computer tool.
const computerUseSubagentSystemPrompt = `You are a computer use agent with access to a desktop environment. You can see the screen, move the mouse, click, type, scroll, and drag.

Your primary tool is the "computer" tool which lets you interact with the desktop. After every action you take, you will receive a screenshot showing the current state of the screen. Use these screenshots to verify your actions and plan next steps.

Guidelines:
- Always start by taking a screenshot to see the current state of the desktop.
- Be precise with coordinates when clicking or typing.
- Wait for UI elements to load before interacting with them.
- If an action doesn't produce the expected result, try alternative approaches.
- Report what you accomplished when done.`

type spawnAgentArgs struct {
	Prompt string `json:"prompt"`
	Title  string `json:"title,omitempty"`
}

type spawnComputerUseAgentArgs struct {
	Prompt string `json:"prompt"`
	Title  string `json:"title,omitempty"`
}

type waitAgentArgs struct {
	ChatID         string `json:"chat_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type messageAgentArgs struct {
	ChatID    string `json:"chat_id"`
	Message   string `json:"message"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

type closeAgentArgs struct {
	ChatID string `json:"chat_id"`
}

// isAnthropicConfigured reports whether an Anthropic API key is
// available, either from static provider keys or from the database.
func (p *Server) isAnthropicConfigured(ctx context.Context) bool {
	if p.providerAPIKeys.APIKey("anthropic") != "" {
		return true
	}
	dbProviders, err := p.configCache.EnabledProviders(ctx)
	if err != nil {
		return false
	}
	for _, prov := range dbProviders {
		if chatprovider.NormalizeProvider(prov.Provider) == "anthropic" && strings.TrimSpace(prov.APIKey) != "" {
			return true
		}
	}
	return false
}

func (p *Server) isDesktopEnabled(ctx context.Context) bool {
	enabled, err := p.db.GetChatDesktopEnabled(ctx)
	if err != nil {
		return false
	}
	return enabled
}

func (p *Server) resolveExploreSubagentModelConfigID(
	ctx context.Context,
	ownerID uuid.UUID,
	fallback uuid.UUID,
) (uuid.UUID, error) {
	//nolint:gocritic // Chatd needs its scoped deployment-config read access here.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := p.db.GetChatExploreModelOverride(chatdCtx)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get Explore model override: %w", err)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	configuredModelConfigID, err := uuid.Parse(trimmed)
	if err != nil {
		p.logger.Warn(ctx,
			"invalid Explore model override, falling back to current turn model",
			slog.F("raw_model_config_id", trimmed),
			slog.Error(err),
		)
		return fallback, nil
	}
	modelConfig, err := p.db.GetEnabledChatModelConfigByID(
		chatdCtx,
		configuredModelConfigID,
	)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			p.logger.Warn(ctx,
				"explore model override is unavailable, falling back to current turn model",
				slog.F("model_config_id", configuredModelConfigID),
			)
			return fallback, nil
		}
		return uuid.Nil, xerrors.Errorf("get enabled chat model config by id: %w", err)
	}
	providerName, _, err := chatprovider.ResolveModelWithProviderHint(
		modelConfig.Model,
		modelConfig.Provider,
	)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("resolve Explore model provider: %w", err)
	}
	providerKeys, err := p.resolveUserProviderAPIKeys(ctx, ownerID)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	if providerKeys.APIKey(providerName) == "" {
		p.logger.Warn(ctx,
			"explore model override credentials are unavailable, falling back to current turn model",
			slog.F("model_config_id", configuredModelConfigID),
			slog.F("provider", providerName),
		)
		return fallback, nil
	}
	return modelConfig.ID, nil
}

func (p *Server) subagentTools(
	ctx context.Context,
	currentChat func() database.Chat,
	currentModelConfigID uuid.UUID,
) []fantasy.AgentTool {
	var planMode database.NullChatPlanMode
	if currentChat != nil {
		planMode = currentChat().PlanMode
	}

	spawnAgentDescription := "Spawn a delegated child agent to work on a clearly scoped, " +
		"independent task in parallel. Use this when the task is " +
		"self-contained and would benefit from a separate agent " +
		"(e.g. fixing a specific bug, writing a single module, " +
		"running a migration). Do NOT use for simple or quick " +
		"operations you can handle directly with execute, " +
		"read_file, or write_file. For read-only investigation and " +
		"codebase discovery, prefer spawn_explore_agent instead. " +
		"Reserve writable subagents for tasks that require " +
		"intellectual work such as code analysis, writing new " +
		"code, or complex refactoring. Be careful when running " +
		"parallel subagents: if two subagents modify the same " +
		"files they will conflict with each other, so ensure " +
		"parallel subagent tasks are independent. " +
		"The child agent receives the same workspace tools but " +
		"cannot spawn its own subagents. After spawning, use " +
		"wait_agent to collect the result."
	if planMode.Valid && planMode.ChatPlanMode == database.ChatPlanModePlan {
		spawnAgentDescription += " During plan mode, spawned agents may use shell commands for exploration, such as cloning repositories, searching code, and running inspection commands, but they must not implement changes or intentionally modify workspace files."
	}
	spawnExploreAgentDescription := "Spawn a read-only delegated child agent for discovery, code reading, and system understanding. Use this when you need investigation, tracing, codebase research, or architecture discovery without intentionally modifying workspace files. The child agent cannot spawn its own subagents and has a restricted toolset focused on reading files and inspection commands. After spawning, use wait_agent to collect the result."

	tools := []fantasy.AgentTool{
		fantasy.NewAgentTool(
			"spawn_agent",
			spawnAgentDescription,
			func(ctx context.Context, args spawnAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				parent := currentChat()
				if parent.ParentChatID.Valid {
					return fantasy.NewTextErrorResponse("delegated chats cannot create child subagents"), nil
				}

				parent, err := p.db.GetChatByID(ctx, parent.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				childChat, err := p.createChildSubagentChatWithOptions(
					ctx,
					parent,
					args.Prompt,
					args.Title,
					childSubagentChatOptions{},
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id": childChat.ID.String(),
					"title":   childChat.Title,
					"status":  string(childChat.Status),
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"spawn_explore_agent",
			spawnExploreAgentDescription,
			func(ctx context.Context, args spawnAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				parent := currentChat()
				if parent.ParentChatID.Valid {
					return fantasy.NewTextErrorResponse("delegated chats cannot create child subagents"), nil
				}

				parent, err := p.db.GetChatByID(ctx, parent.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				modelConfigID, err := p.resolveExploreSubagentModelConfigID(
					ctx,
					parent.OwnerID,
					currentModelConfigID,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				// Explore subagents operate independently of planning.
				// Clear plan mode to prevent the child from inheriting
				// parent planning behavior.
				clearPlanMode := database.NullChatPlanMode{}
				childChat, err := p.createChildSubagentChatWithOptions(
					ctx,
					parent,
					args.Prompt,
					args.Title,
					childSubagentChatOptions{
						chatMode: database.NullChatMode{
							ChatMode: database.ChatModeExplore,
							Valid:    true,
						},
						modelConfigIDOverride: &modelConfigID,
						planModeOverride:      &clearPlanMode,
					},
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id": childChat.ID.String(),
					"title":   childChat.Title,
					"status":  string(childChat.Status),
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"wait_agent",
			"Wait until a spawned child agent finishes its task. "+
				"Returns the agent's final response and status. "+
				"Call this after spawn_agent, spawn_explore_agent, or "+
				"spawn_computer_use_agent to collect the result before "+
				"continuing your own work.",
			func(ctx context.Context, args waitAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				timeout := defaultSubagentWaitTimeout
				if args.TimeoutSeconds != nil {
					timeout = time.Duration(*args.TimeoutSeconds) * time.Second
				}

				parent := currentChat()

				// Authorize: the target chat must be a descendant
				// of the current (parent) chat.
				isDescendant, descErr := isSubagentDescendant(ctx, p.db, parent.ID, targetChatID)
				if descErr != nil {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("failed to verify subagent relationship: %v", descErr)), nil
				}
				if !isDescendant {
					return fantasy.NewTextErrorResponse(
						"target chat is not a subagent of the current chat"), nil
				}

				// Check if the target is a computer_use subagent
				// and start a desktop recording. Failures are
				// best-effort warnings — recording never blocks
				// the wait_agent flow.
				var recordingID string
				var agentConn workspacesdk.AgentConn

				targetChatInfo, lookupErr := p.db.GetChatByID(ctx, targetChatID)
				if lookupErr != nil && !xerrors.Is(lookupErr, sql.ErrNoRows) {
					p.logger.Warn(ctx, "unexpected error looking up chat for recording",
						slog.F("chat_id", targetChatID),
						slog.Error(lookupErr),
					)
				}
				isComputerUseChat := lookupErr == nil && targetChatInfo.Mode.Valid &&
					targetChatInfo.Mode.ChatMode == database.ChatModeComputerUse &&
					targetChatInfo.AgentID.Valid
				canRecord := isComputerUseChat && p.agentConnFn != nil

				if canRecord {
					conn, closeFn, connErr := p.agentConnFn(ctx, targetChatInfo.AgentID.UUID)
					if connErr == nil {
						agentConn = conn
						defer closeFn()

						recordingID = targetChatID.String()
						startErr := conn.StartDesktopRecording(ctx,
							workspacesdk.StartDesktopRecordingRequest{RecordingID: recordingID})
						if startErr != nil {
							p.logger.Warn(ctx, "failed to start desktop recording",
								slog.Error(startErr))
							recordingID = "" // Don't try to stop.
						}
					} else {
						p.logger.Warn(ctx, "failed to get agent conn for recording",
							slog.Error(connErr))
					}
				}

				targetChat, report, awaitErr := p.awaitSubagentCompletion(
					ctx, parent.ID, targetChatID, timeout,
				)

				// On timeout/error, leave the recording running on
				// the agent so the next wait_agent call continues
				// it seamlessly.
				if awaitErr != nil {
					return fantasy.NewTextErrorResponse(awaitErr.Error()), nil
				}

				// Only stop and store the recording on success.
				var recResult recordingResult
				if recordingID != "" && agentConn != nil {
					// Use a fresh context for cleanup so a canceled
					// parent context doesn't prevent recording storage.
					stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
					defer stopCancel()
					recResult = p.stopAndStoreRecording(stopCtx, agentConn,
						recordingID, parent.OwnerID, parent.WorkspaceID, parent.ID)
				}
				resp := map[string]any{
					"chat_id": targetChatID.String(),
					"title":   targetChat.Title,
					"report":  report,
					"status":  string(targetChat.Status),
				}
				if recResult.recordingFileID != "" {
					resp["recording_file_id"] = recResult.recordingFileID
				}
				if recResult.thumbnailFileID != "" {
					resp["thumbnail_file_id"] = recResult.thumbnailFileID
				}
				return toolJSONResponse(resp), nil
			},
		),
		fantasy.NewAgentTool(
			"message_agent",
			"Send a follow-up message to a previously spawned child "+
				"agent. Use this to provide additional instructions, "+
				"corrections, or context to a running or completed "+
				"agent. After sending, use wait_agent to collect the "+
				"updated response.",
			func(ctx context.Context, args messageAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				parent := currentChat()
				busyBehavior := SendMessageBusyBehaviorQueue
				if args.Interrupt {
					busyBehavior = SendMessageBusyBehaviorInterrupt
				}
				targetChat, err := p.sendSubagentMessage(
					ctx,
					parent.ID,
					targetChatID,
					args.Message,
					busyBehavior,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id":     targetChatID.String(),
					"title":       targetChat.Title,
					"status":      string(targetChat.Status),
					"interrupted": args.Interrupt,
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"close_agent",
			"Immediately stop a spawned child agent. Use this to "+
				"cancel a subagent that is stuck, no longer needed, "+
				"or working on the wrong approach.",
			func(ctx context.Context, args closeAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				parent := currentChat()
				targetChat, err := p.closeSubagent(
					ctx,
					parent.ID,
					targetChatID,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id":    targetChatID.String(),
					"title":      targetChat.Title,
					"terminated": true,
					"status":     string(targetChat.Status),
				}), nil
			},
		),
	}

	// Only include the computer use tool when an Anthropic
	// provider is configured and desktop is enabled.
	if p.isAnthropicConfigured(ctx) && p.isDesktopEnabled(ctx) {
		tools = append(tools, fantasy.NewAgentTool(
			"spawn_computer_use_agent",
			"Spawn a dedicated computer use agent that can see the desktop "+
				"(take screenshots) and interact with it (mouse, keyboard, "+
				"scroll). The agent runs on a model optimized for computer "+
				"use and has the same workspace tools as a standard subagent "+
				"plus the native Anthropic computer tool. Use this for tasks "+
				"that require visual interaction with a desktop GUI (e.g. "+
				"browser automation, GUI testing, visual inspection). After "+
				"spawning, use wait_agent to collect the result.",
			func(ctx context.Context, args spawnComputerUseAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				parent := currentChat()
				if parent.ParentChatID.Valid {
					return fantasy.NewTextErrorResponse("delegated chats cannot create child subagents"), nil
				}

				parent, err := p.db.GetChatByID(ctx, parent.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				childChat, err := p.createChildSubagentChatWithOptions(
					ctx,
					parent,
					args.Prompt,
					args.Title,
					childSubagentChatOptions{
						chatMode: database.NullChatMode{
							ChatMode: database.ChatModeComputerUse,
							Valid:    true,
						},
						systemPrompt: computerUseSubagentSystemPrompt + "\n\n" + strings.TrimSpace(args.Prompt),
					},
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id": childChat.ID.String(),
					"title":   childChat.Title,
					"status":  string(childChat.Status),
				}), nil
			},
		))
	}

	return tools
}

func parseSubagentToolChatID(raw string) (uuid.UUID, error) {
	chatID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, xerrors.New("chat_id must be a valid UUID")
	}
	return chatID, nil
}

type childSubagentChatOptions struct {
	chatMode              database.NullChatMode
	systemPrompt          string
	modelConfigIDOverride *uuid.UUID
	planModeOverride      *database.NullChatPlanMode
}

func (p *Server) createChildSubagentChat(
	ctx context.Context,
	parent database.Chat,
	prompt string,
	title string,
) (database.Chat, error) {
	return p.createChildSubagentChatWithOptions(ctx, parent, prompt, title, childSubagentChatOptions{})
}

func (p *Server) createChildSubagentChatWithOptions(
	ctx context.Context,
	parent database.Chat,
	prompt string,
	title string,
	opts childSubagentChatOptions,
) (database.Chat, error) {
	if parent.ParentChatID.Valid {
		return database.Chat{}, xerrors.New("delegated chats cannot create child subagents")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return database.Chat{}, xerrors.New("prompt is required")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = subagentFallbackChatTitle(prompt)
	}

	rootChatID := parent.ID
	if parent.RootChatID.Valid {
		rootChatID = parent.RootChatID.UUID
	}

	modelConfigID := parent.LastModelConfigID
	if opts.modelConfigIDOverride != nil {
		modelConfigID = *opts.modelConfigIDOverride
	}
	if modelConfigID == uuid.Nil {
		return database.Chat{}, xerrors.New("model config is required")
	}

	childPlanMode := parent.PlanMode
	if opts.planModeOverride != nil {
		childPlanMode = *opts.planModeOverride
	}

	mcpServerIDs := parent.MCPServerIDs
	if mcpServerIDs == nil {
		mcpServerIDs = []uuid.UUID{}
	}

	labelsJSON, err := json.Marshal(database.StringMap{})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal labels: %w", err)
	}
	childSystemPrompt := SanitizePromptText(opts.systemPrompt)
	// Resolve the deployment prompt before opening the transaction so
	// child chat creation does not hold one DB connection while waiting
	// for another pool checkout.
	deploymentPrompt := p.resolveDeploymentSystemPrompt(ctx)

	var child database.Chat
	txErr := p.db.InTx(func(tx database.Store) error {
		if limitErr := p.checkUsageLimit(ctx, tx, parent.OwnerID, uuid.NullUUID{UUID: parent.OrganizationID, Valid: true}); limitErr != nil {
			return limitErr
		}

		insertedChat, err := tx.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    parent.OrganizationID,
			OwnerID:           parent.OwnerID,
			WorkspaceID:       parent.WorkspaceID,
			BuildID:           parent.BuildID,
			AgentID:           parent.AgentID,
			ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: rootChatID, Valid: true},
			LastModelConfigID: modelConfigID,
			Title:             title,
			Mode:              opts.chatMode,
			PlanMode:          childPlanMode,
			ClientType:        parent.ClientType,
			Status:            database.ChatStatusPending,
			MCPServerIDs:      mcpServerIDs,
			Labels: pqtype.NullRawMessage{
				RawMessage: labelsJSON,
				Valid:      true,
			},
			DynamicTools: pqtype.NullRawMessage{},
		})
		if err != nil {
			return xerrors.Errorf("insert child chat: %w", err)
		}

		workspaceAwareness := "There is no workspace associated with this chat yet. Create one using the create_workspace tool before using workspace tools like execute, read_file, write_file, etc."
		if insertedChat.WorkspaceID.Valid {
			workspaceAwareness = "This chat is attached to a workspace. You can use workspace tools like execute, read_file, write_file, etc."
		}
		workspaceAwarenessContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(workspaceAwareness),
		})
		if err != nil {
			return xerrors.Errorf("marshal workspace awareness: %w", err)
		}
		userContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)})
		if err != nil {
			return xerrors.Errorf("marshal initial user content: %w", err)
		}

		systemParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
			ChatID: insertedChat.ID,
		}
		if deploymentPrompt != "" {
			deploymentContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageText(deploymentPrompt),
			})
			if err != nil {
				return xerrors.Errorf("marshal deployment system prompt: %w", err)
			}
			appendChatMessage(&systemParams, newChatMessage(
				database.ChatMessageRoleSystem,
				deploymentContent,
				database.ChatMessageVisibilityModel,
				modelConfigID,
				chatprompt.CurrentContentVersion,
			))
		}
		if childSystemPrompt != "" {
			childSystemPromptContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageText(childSystemPrompt),
			})
			if err != nil {
				return xerrors.Errorf("marshal child system prompt: %w", err)
			}
			appendChatMessage(&systemParams, newChatMessage(
				database.ChatMessageRoleSystem,
				childSystemPromptContent,
				database.ChatMessageVisibilityModel,
				modelConfigID,
				chatprompt.CurrentContentVersion,
			))
		}
		appendChatMessage(&systemParams, newChatMessage(
			database.ChatMessageRoleSystem,
			workspaceAwarenessContent,
			database.ChatMessageVisibilityModel,
			modelConfigID,
			chatprompt.CurrentContentVersion,
		))
		if _, err := tx.InsertChatMessages(ctx, systemParams); err != nil {
			return xerrors.Errorf("insert initial child system messages: %w", err)
		}

		child = insertedChat

		// Copy persisted context before the initial child prompt so the
		// child cannot be acquired until its inherited context is in
		// place. signalWake runs only after commit.
		copiedContextParts, err := copyParentContextMessages(ctx, p.logger, tx, parent, child)
		if err != nil {
			return xerrors.Errorf("copy parent context messages: %w", err)
		}
		if err := updateChildLastInjectedContext(ctx, p.logger, tx, child.ID, copiedContextParts); err != nil {
			return xerrors.Errorf("update child injected context: %w", err)
		}

		userParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
			ChatID: insertedChat.ID,
		}
		appendChatMessage(&userParams, newChatMessage(
			database.ChatMessageRoleUser,
			userContent,
			database.ChatMessageVisibilityBoth,
			modelConfigID,
			chatprompt.CurrentContentVersion,
		).withCreatedBy(parent.OwnerID))
		if _, err := tx.InsertChatMessages(ctx, userParams); err != nil {
			return xerrors.Errorf("insert initial child user message: %w", err)
		}

		return nil
	}, nil)
	if txErr != nil {
		return database.Chat{}, xerrors.Errorf("create child chat: %w", txErr)
	}

	p.publishChatPubsubEvent(child, codersdk.ChatWatchEventKindCreated, nil)
	p.signalWake()
	return child, nil
}

// copyParentContextMessages reads persisted context-file and skill
// messages from the parent chat and inserts copies into the child
// chat. This ensures sub-agents inherit the same instruction and
// skill context as their parent without independently re-fetching
// from the agent.
func copyParentContextMessages(
	ctx context.Context,
	logger slog.Logger,
	store database.Store,
	parent database.Chat,
	child database.Chat,
) ([]codersdk.ChatMessagePart, error) {
	parentMessages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  parent.ID,
		AfterID: 0,
	})
	if err != nil {
		return nil, xerrors.Errorf("get parent messages: %w", err)
	}

	var (
		copiedParts      []codersdk.ChatMessagePart
		copiedRole       database.ChatMessageRole
		copiedVisibility database.ChatMessageVisibility
		copiedVersion    int16
	)
	for _, msg := range parentMessages {
		if !msg.Content.Valid {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			logger.Warn(ctx, "failed to unmarshal parent context message",
				slog.F("parent_chat_id", parent.ID),
				slog.F("message_id", msg.ID),
				slog.Error(err),
			)
			continue
		}

		messageContextParts := FilterContextParts(parts, true)
		if len(messageContextParts) == 0 {
			continue
		}
		if copiedParts == nil {
			copiedRole = msg.Role
			copiedVisibility = msg.Visibility
			copiedVersion = msg.ContentVersion
		}
		copiedParts = append(copiedParts, messageContextParts...)
	}
	if len(copiedParts) == 0 {
		return nil, nil
	}

	copiedParts = FilterContextPartsToLatestAgent(copiedParts)
	filteredContent, err := chatprompt.MarshalParts(copiedParts)
	if err != nil {
		return nil, xerrors.Errorf("marshal filtered context parts: %w", err)
	}

	msgParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
		ChatID: child.ID,
	}
	appendChatMessage(&msgParams, newChatMessage(
		copiedRole,
		filteredContent,
		copiedVisibility,
		child.LastModelConfigID,
		copiedVersion,
	))
	if _, err := store.InsertChatMessages(ctx, msgParams); err != nil {
		return nil, xerrors.Errorf("insert context message: %w", err)
	}

	return copiedParts, nil
}

func updateChildLastInjectedContext(
	ctx context.Context,
	logger slog.Logger,
	store database.Store,
	chatID uuid.UUID,
	parts []codersdk.ChatMessagePart,
) error {
	parts = FilterContextPartsToLatestAgent(parts)
	param, err := BuildLastInjectedContext(parts)
	if err != nil {
		logger.Warn(ctx, "failed to marshal inherited injected context",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return xerrors.Errorf("marshal inherited injected context: %w", err)
	}
	if _, err := store.UpdateChatLastInjectedContext(ctx, database.UpdateChatLastInjectedContextParams{
		ID:                  chatID,
		LastInjectedContext: param,
	}); err != nil {
		logger.Warn(ctx, "failed to update inherited injected context",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return xerrors.Errorf("update inherited injected context: %w", err)
	}

	return nil
}

func (p *Server) sendSubagentMessage(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	message string,
	busyBehavior SendMessageBusyBehavior,
) (database.Chat, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return database.Chat{}, xerrors.New("message is required")
	}

	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, err
	}
	if !isDescendant {
		return database.Chat{}, ErrSubagentNotDescendant
	}

	// Look up the target chat to get the owner for CreatedBy.
	targetChat, err := p.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get target chat: %w", err)
	}

	sendResult, err := p.SendMessage(ctx, SendMessageOptions{
		ChatID:       targetChatID,
		CreatedBy:    targetChat.OwnerID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText(message)},
		BusyBehavior: busyBehavior,
	})
	if err != nil {
		return database.Chat{}, err
	}

	return sendResult.Chat, nil
}

func (p *Server) awaitSubagentCompletion(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	timeout time.Duration,
) (database.Chat, string, error) {
	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, "", err
	}
	if !isDescendant {
		return database.Chat{}, "", ErrSubagentNotDescendant
	}

	// Check immediately before entering the poll loop.
	targetChat, report, done, checkErr := p.checkSubagentCompletion(ctx, targetChatID)
	if checkErr != nil {
		return database.Chat{}, "", checkErr
	}
	if done {
		return handleSubagentDone(targetChat, report)
	}

	if timeout <= 0 {
		timeout = defaultSubagentWaitTimeout
	}
	timer := p.clock.NewTimer(timeout, "chatd", "subagent_await")
	defer timer.Stop()

	// When pubsub is available, subscribe for fast status
	// notifications and use a less aggressive fallback poll.
	// Without pubsub (single-instance / in-memory) fall back
	// to the original 200ms polling.
	pollInterval := subagentAwaitPollInterval
	var notifyCh <-chan struct{}
	if p.pubsub != nil {
		pollInterval = subagentAwaitFallbackPoll
		ch := make(chan struct{}, 1)
		notifyCh = ch
		cancel, subErr := p.pubsub.SubscribeWithErr(
			coderdpubsub.ChatStreamNotifyChannel(targetChatID),
			func(_ context.Context, _ []byte, _ error) {
				// Non-blocking send so we never stall the
				// pubsub dispatch goroutine.
				select {
				case ch <- struct{}{}:
				default:
				}
			},
		)
		if subErr == nil {
			defer cancel()
		} else {
			// Subscription failed; fall back to fast polling.
			pollInterval = subagentAwaitPollInterval
			notifyCh = nil
		}
	}

	ticker := p.clock.NewTicker(pollInterval, "chatd", "subagent_poll")
	defer ticker.Stop()

	for {
		select {
		case <-notifyCh:
		case <-ticker.C:
		case <-timer.C:
			return database.Chat{}, "", xerrors.New("timed out waiting for delegated subagent completion")
		case <-ctx.Done():
			return database.Chat{}, "", ctx.Err()
		}

		targetChat, report, done, checkErr = p.checkSubagentCompletion(ctx, targetChatID)
		if checkErr != nil {
			return database.Chat{}, "", checkErr
		}
		if done {
			return handleSubagentDone(targetChat, report)
		}
	}
}

// handleSubagentDone translates a completed subagent check into the
// appropriate return value, surfacing error-status chats as errors.
func handleSubagentDone(
	chat database.Chat,
	report string,
) (database.Chat, string, error) {
	if chat.Status == database.ChatStatusError {
		reason := strings.TrimSpace(report)
		if reason == "" {
			reason = "agent reached error status"
		}
		return database.Chat{}, "", xerrors.New(reason)
	}
	return chat, report, nil
}

func (p *Server) closeSubagent(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
) (database.Chat, error) {
	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, err
	}
	if !isDescendant {
		return database.Chat{}, ErrSubagentNotDescendant
	}

	targetChat, err := p.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get target chat: %w", err)
	}

	if targetChat.Status == database.ChatStatusWaiting {
		return targetChat, nil
	}

	updatedChat := p.InterruptChat(ctx, targetChat)
	if updatedChat.Status != database.ChatStatusWaiting {
		return database.Chat{}, xerrors.New("set target chat waiting")
	}
	return updatedChat, nil
}

func (p *Server) checkSubagentCompletion(
	ctx context.Context,
	chatID uuid.UUID,
) (database.Chat, string, bool, error) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return database.Chat{}, "", false, xerrors.Errorf("get chat: %w", err)
	}

	if chat.Status == database.ChatStatusPending || chat.Status == database.ChatStatusRunning {
		return database.Chat{}, "", false, nil
	}

	report, err := latestSubagentAssistantMessage(ctx, p.db, chatID)
	if err != nil {
		return database.Chat{}, "", false, err
	}

	return chat, report, true, nil
}

func latestSubagentAssistantMessage(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
) (string, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: 0,
	})
	if err != nil {
		return "", xerrors.Errorf("get chat messages: %w", err)
	}

	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != database.ChatMessageRoleAssistant ||
			message.Visibility == database.ChatMessageVisibilityModel {
			continue
		}

		content, parseErr := chatprompt.ParseContent(message)
		if parseErr != nil {
			continue
		}
		text := strings.TrimSpace(contentBlocksToText(content))
		if text == "" {
			continue
		}
		return text, nil
	}

	return "", nil
}

// isSubagentDescendant reports whether targetChatID is a descendant
// of ancestorChatID by walking up the parent chain from the target.
// This is O(depth) DB queries instead of O(nodes) BFS.
func isSubagentDescendant(
	ctx context.Context,
	store database.Store,
	ancestorChatID uuid.UUID,
	targetChatID uuid.UUID,
) (bool, error) {
	if ancestorChatID == targetChatID {
		return false, nil
	}

	currentID := targetChatID
	visited := map[uuid.UUID]struct{}{} // cycle protection
	for {
		if _, seen := visited[currentID]; seen {
			return false, nil
		}
		visited[currentID] = struct{}{}

		chat, err := store.GetChatByID(ctx, currentID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return false, nil // chain broken; not a confirmed descendant
			}
			return false, xerrors.Errorf("get chat %s: %w", currentID, err)
		}
		if !chat.ParentChatID.Valid {
			return false, nil // reached root without finding ancestor
		}
		if chat.ParentChatID.UUID == ancestorChatID {
			return true, nil
		}
		currentID = chat.ParentChatID.UUID
	}
}

func subagentFallbackChatTitle(message string) string {
	const maxWords = 6
	const maxRunes = 80

	words := strings.Fields(message)
	if len(words) == 0 {
		return "New Chat"
	}

	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}

	title := strings.Join(words, " ")
	if truncated {
		title += "..."
	}

	return subagentTruncateRunes(title, maxRunes)
}

func subagentTruncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}

	return string(runes[:maxRunes])
}

func toolJSONResponse(result map[string]any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
}
