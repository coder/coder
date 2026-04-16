package chatd

import (
	"context"
	"errors"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatcost"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// ErrStaleLease reports that the chat lease epoch changed before a step could
// be persisted.
var ErrStaleLease = xerrors.New("chat lease epoch changed before step persist")

// PersistChatStepParams contains the inputs required to persist a chat loop
// step without reaching back into runChat state.
type PersistChatStepParams struct {
	ChatID             uuid.UUID
	ExpectedLeaseEpoch int64
	ExpectedWorkerID   uuid.UUID
	Step               chatloop.PersistedStep
	CallConfig         codersdk.ChatModelCallConfig
	ToolNameToConfigID map[string]uuid.UUID
	ModelConfigID      uuid.UUID
}

// PersistChatStepResult contains the state that runChat surfaces after a step
// is durably persisted.
type PersistChatStepResult struct {
	AssistantContentPersisted bool
	FinalAssistantText        string
	PendingDynamicToolCalls   []chatloop.PendingToolCall
}

// ReloadChatMessagesParams contains the prompt-injection state needed to
// rebuild prompt messages after compaction or retries.
type ReloadChatMessagesParams struct {
	ChatID                     uuid.UUID
	Instruction                string
	Skills                     []chattool.SkillMeta
	ResolvedUserPrompt         string
	ResolveUserPrompt          func(context.Context) string
	IncludeSubagentInstruction bool
	ChainMode                  ResolvedChainMode
	ChainModeActive            bool
	PlanContext                systemPromptBehaviorContext
	PlanPathBlock              string
	ResolvePlanPathBlock       func(context.Context) string
	OnInstructionResolved      func(string)
	Logger                     slog.Logger
}

func annotateMessagePartWithConfigID(
	part codersdk.ChatMessagePart,
	toolNameToConfigID map[string]uuid.UUID,
) codersdk.ChatMessagePart {
	if part.ToolName == "" || len(toolNameToConfigID) == 0 {
		return part
	}
	configID, ok := toolNameToConfigID[part.ToolName]
	if !ok {
		return part
	}
	part.MCPServerConfigID = uuid.NullUUID{UUID: configID, Valid: true}
	return part
}

// PublishChatMessagePart publishes a streamed chat message part after adding
// any known MCP server configuration metadata.
func (p *Server) PublishChatMessagePart(
	chatID uuid.UUID,
	role codersdk.ChatMessageRole,
	part codersdk.ChatMessagePart,
	toolNameToConfigID map[string]uuid.UUID,
) {
	p.publishMessagePart(chatID, role, annotateMessagePartWithConfigID(part, toolNameToConfigID))
}

// PersistChatStep persists one chat loop step and publishes any inserted
// messages after the transaction commits.
func (p *Server) PersistChatStep(
	persistCtx context.Context,
	params PersistChatStepParams,
) (PersistChatStepResult, error) {
	result := PersistChatStepResult{
		PendingDynamicToolCalls: params.Step.PendingDynamicToolCalls,
	}

	if persistCtx.Err() != nil {
		if errors.Is(context.Cause(persistCtx), chatloop.ErrInterrupted) {
			return result, chatloop.ErrInterrupted
		}
		return result, persistCtx.Err()
	}

	var assistantBlocks []fantasy.Content
	var toolResults []fantasy.ToolResultContent
	for _, block := range params.Step.Content {
		if tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
			if !tr.ProviderExecuted {
				toolResults = append(toolResults, tr)
				continue
			}
		}
		if trPtr, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && trPtr != nil {
			if !trPtr.ProviderExecuted {
				toolResults = append(toolResults, *trPtr)
				continue
			}
		}
		assistantBlocks = append(assistantBlocks, block)
	}

	var assistantContent pqtype.NullRawMessage
	if len(assistantBlocks) > 0 {
		sdkParts := make([]codersdk.ChatMessagePart, 0, len(assistantBlocks))
		for _, block := range assistantBlocks {
			part := annotateMessagePartWithConfigID(
				chatprompt.PartFromContent(block),
				params.ToolNameToConfigID,
			)
			if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolCallID != "" && params.Step.ToolCallCreatedAt != nil {
				if ts, ok := params.Step.ToolCallCreatedAt[part.ToolCallID]; ok {
					part.CreatedAt = &ts
				}
			}
			if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolCallID != "" && params.Step.ToolResultCreatedAt != nil {
				if ts, ok := params.Step.ToolResultCreatedAt[part.ToolCallID]; ok {
					part.CreatedAt = &ts
				}
			}
			sdkParts = append(sdkParts, part)
		}
		result.AssistantContentPersisted = true
		result.FinalAssistantText = strings.TrimSpace(contentBlocksToText(sdkParts))
		var marshalErr error
		assistantContent, marshalErr = chatprompt.MarshalParts(sdkParts)
		if marshalErr != nil {
			return result, xerrors.Errorf("marshal assistant content: %w", marshalErr)
		}
	}

	toolResultContents := make([]pqtype.NullRawMessage, len(toolResults))
	for i, tr := range toolResults {
		trPart := annotateMessagePartWithConfigID(
			chatprompt.PartFromContent(tr),
			params.ToolNameToConfigID,
		)
		if trPart.ToolCallID != "" && params.Step.ToolResultCreatedAt != nil {
			if ts, ok := params.Step.ToolResultCreatedAt[trPart.ToolCallID]; ok {
				trPart.CreatedAt = &ts
			}
		}
		var marshalErr error
		toolResultContents[i], marshalErr = chatprompt.MarshalParts([]codersdk.ChatMessagePart{trPart})
		if marshalErr != nil {
			return result, xerrors.Errorf("marshal tool result %d: %w", i, marshalErr)
		}
	}

	hasUsage := params.Step.Usage != (fantasy.Usage{})
	usageForCost := fantasyUsageToChatMessageUsage(params.Step.Usage)
	totalCostMicros := chatcost.CalculateTotalCostMicros(usageForCost, params.CallConfig.Cost)

	var insertedMessages []database.ChatMessage
	err := p.db.InTx(func(tx database.Store) error {
		lockedChat, lockErr := tx.GetChatByIDForUpdate(persistCtx, params.ChatID)
		if lockErr != nil {
			return xerrors.Errorf("lock chat for persist: %w", lockErr)
		}
		if lockedChat.LeaseEpoch != params.ExpectedLeaseEpoch {
			return ErrStaleLease
		}
		if !lockedChat.WorkerID.Valid || lockedChat.WorkerID.UUID != params.ExpectedWorkerID {
			if lockedChat.Status != database.ChatStatusWaiting {
				return chatloop.ErrInterrupted
			}
		}

		stepParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
			ChatID: params.ChatID,
		}

		var contextLimit int64
		if params.Step.ContextLimit.Valid {
			contextLimit = params.Step.ContextLimit.Int64
		}

		var runtimeMs int64
		if params.Step.Runtime > 0 {
			runtimeMs = params.Step.Runtime.Milliseconds()
		}

		var totalCostVal int64
		if totalCostMicros != nil {
			totalCostVal = *totalCostMicros
		}

		var inputTokens, outputTokens, totalTokens int64
		var reasoningTokens, cacheCreationTokens, cacheReadTokens int64
		if hasUsage {
			inputTokens = params.Step.Usage.InputTokens
			outputTokens = params.Step.Usage.OutputTokens
			totalTokens = params.Step.Usage.TotalTokens
			reasoningTokens = params.Step.Usage.ReasoningTokens
			cacheCreationTokens = params.Step.Usage.CacheCreationTokens
			cacheReadTokens = params.Step.Usage.CacheReadTokens
		}

		if assistantContent.Valid {
			appendChatMessage(&stepParams, newChatMessage(
				database.ChatMessageRoleAssistant,
				assistantContent,
				database.ChatMessageVisibilityBoth,
				params.ModelConfigID,
				chatprompt.CurrentContentVersion,
			).withUsage(
				inputTokens, outputTokens, totalTokens,
				reasoningTokens, cacheCreationTokens, cacheReadTokens,
			).withContextLimit(contextLimit).
				withTotalCostMicros(totalCostVal).
				withRuntimeMs(runtimeMs).
				withProviderResponseID(params.Step.ProviderResponseID))
		}

		for _, resultContent := range toolResultContents {
			appendChatMessage(&stepParams, newChatMessage(
				database.ChatMessageRoleTool,
				resultContent,
				database.ChatMessageVisibilityBoth,
				params.ModelConfigID,
				chatprompt.CurrentContentVersion,
			))
		}

		if len(stepParams.Role) > 0 {
			inserted, insertErr := tx.InsertChatMessages(persistCtx, stepParams)
			if insertErr != nil {
				return xerrors.Errorf("insert step messages: %w", insertErr)
			}
			insertedMessages = append(insertedMessages, inserted...)
		}

		return nil
	}, nil)
	if err != nil {
		return result, xerrors.Errorf("persist step transaction: %w", err)
	}

	for _, msg := range insertedMessages {
		p.publishMessage(params.ChatID, msg)
	}

	return result, nil
}

// ReloadChatMessages reloads persisted chat messages and re-applies the system
// prompt injections used during a running chat loop.
func (p *Server) ReloadChatMessages(
	reloadCtx context.Context,
	params ReloadChatMessagesParams,
) ([]fantasy.Message, error) {
	reloadedMsgs, err := p.db.GetChatMessagesForPromptByChatID(reloadCtx, params.ChatID)
	if err != nil {
		return nil, xerrors.Errorf("reload chat messages: %w", err)
	}
	reloadedPrompt, err := chatprompt.ConvertMessagesWithFiles(reloadCtx, reloadedMsgs, p.chatFileResolver(), params.Logger)
	if err != nil {
		return nil, xerrors.Errorf("convert reloaded messages: %w", err)
	}

	reloadedInstruction := params.Instruction
	if reloadedInstruction == "" {
		reloadedInstruction = instructionFromContextFiles(reloadedMsgs)
	}
	if params.OnInstructionResolved != nil {
		params.OnInstructionResolved(reloadedInstruction)
	}

	reloadedSkills := skillsFromParts(reloadedMsgs)
	if len(reloadedSkills) == 0 {
		reloadedSkills = params.Skills
	}

	reloadUserPrompt := params.ResolvedUserPrompt
	if params.ResolveUserPrompt != nil {
		reloadUserPrompt = params.ResolveUserPrompt(reloadCtx)
	}

	subagentInstruction := ""
	if params.IncludeSubagentInstruction {
		subagentInstruction = defaultSubagentInstruction
	}
	reloadedPrompt = buildSystemPrompt(
		reloadedPrompt,
		subagentInstruction,
		reloadedInstruction,
		reloadedSkills,
		reloadUserPrompt,
		params.PlanContext,
	)

	planPathBlock := params.PlanPathBlock
	if params.ResolvePlanPathBlock != nil {
		planPathBlock = params.ResolvePlanPathBlock(reloadCtx)
	}
	reloadedPrompt = renderPlanPathPrompt(reloadedPrompt, planPathBlock)
	if params.ChainModeActive {
		reloadedPrompt = chatopenai.FilterPromptForChainMode(reloadedPrompt, params.ChainMode.chainModeInfo())
	}
	return reloadedPrompt, nil
}
