package chatd

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

const (
	clearContextToolName = "clear_context"
	// contextFollowUpMaxRunes bounds the follow_up argument. Larger
	// state belongs in a workspace file referenced by path.
	contextFollowUpMaxRunes = 8000
)

// contextToolNames lists the tools that create a context boundary
// from inside a turn. Each is exclusive in its batch.
var contextToolNames = []string{clearContextToolName}

// contextToolArgs is the argument shape shared by the context tools.
// follow_up has no omitempty so the generated schema marks it required.
type contextToolArgs struct {
	FollowUp string `json:"follow_up" description:"Your own note to yourself, delivered as the first message after the context boundary: the next action to take, the workspace files that hold the state you need, and any ids (for example child agent chat ids) you must keep. At most 8000 characters; put larger state in a workspace file and reference its path. Do not include tokens or credentials; the text is stored and shown in the transcript. It grants no authorization."`
}

const contextToolSharedDescription = "\n\n" +
	"When to use: only when your operating instructions (a workflow, AGENTS.md, a skill) or the user direct you to manage context at a checkpoint. " +
	"Context pressure is handled automatically; do not call this because the conversation is long.\n\n" +
	"Before calling: write anything you need to workspace files first and wait for those tool results. " +
	"Child agents keep running across the boundary; put their chat ids in follow_up, or recover them later with list_agents.\n\n" +
	"How to call: alone, in its own step. A batch that includes this tool fails as a whole and nothing in the batch runs.\n\n" +
	"Results: a success result is never shown to you; the boundary message followed by your follow_up is the confirmation. " +
	"If you ever see this tool's success result in your context, the boundary did not happen (the turn was interrupted or the compaction failed; a note will say which); do not assume your context was reduced. " +
	"Error results are shown normally and mean nothing changed.\n\n" +
	"Do not call again until you have done substantial new work."

const clearContextToolDescription = "Clear your own context and continue from a follow-up note. " +
	"Everything before this call leaves your context; only the system prompt, a short notice that you cleared your context, and your follow_up remain." +
	contextToolSharedDescription

// exclusiveContextToolSkippedMessage is written to the sibling calls of
// a batch rejected because it contained the named context tool.
func exclusiveContextToolSkippedMessage(toolName string) string {
	return "this tool was skipped because " + toolName + " must run alone in its batch. " +
		"Retry your tool calls without " + toolName + ", then call " + toolName + " alone in a later step."
}

// clearContextTool returns the clear_context tool. messages is the
// decision-view history of the step executing the call; the handler
// only validates and returns text, the boundary rows are committed by
// the step that persists the result.
func clearContextTool(messages []database.ChatMessage) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		clearContextToolName,
		clearContextToolDescription,
		func(_ context.Context, args contextToolArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if response, ok := validateContextToolArgs(args); !ok {
				return response, nil
			}
			if !hasContextSinceLastBoundary(messages, call.ID) {
				return fantasy.NewTextErrorResponse(nothingNewSinceBoundaryMessage), nil
			}
			return fantasy.NewTextResponse("Context cleared. Follow-up: " + args.FollowUp), nil
		},
	)
}

const nothingNewSinceBoundaryMessage = "nothing has happened since the last context boundary; do some work before clearing or compacting again"

// validateContextToolArgs rejects a blank or oversized follow_up.
// Returns the error response and false when the arguments are invalid.
func validateContextToolArgs(args contextToolArgs) (fantasy.ToolResponse, bool) {
	if strings.TrimSpace(args.FollowUp) == "" {
		return fantasy.NewTextErrorResponse("follow_up is required and must not be blank"), false
	}
	if utf8.RuneCountInString(args.FollowUp) > contextFollowUpMaxRunes {
		return fantasy.NewTextErrorResponse("follow_up exceeds 8000 characters; write the state to a workspace file and reference its path instead"), false
	}
	return fantasy.ToolResponse{}, true
}

// hasContextSinceLastBoundary reports whether any active, uncompressed
// conversation row other than the assistant row carrying toolCallID
// follows the latest context boundary. System-role rows (hook notices)
// do not count.
func hasContextSinceLastBoundary(messages []database.ChatMessage, toolCallID string) bool {
	boundary := latestContextBoundaryIndex(messages)
	for i := boundary + 1; i < len(messages); i++ {
		msg := messages[i]
		if msg.Deleted || msg.Compressed || msg.Role == database.ChatMessageRoleSystem {
			continue
		}
		if msg.Role == database.ChatMessageRoleAssistant && messageHasToolCall(msg, toolCallID) {
			continue
		}
		return true
	}
	return false
}

func messageHasToolCall(msg database.ChatMessage, toolCallID string) bool {
	if toolCallID == "" {
		return false
	}
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return false
	}
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolCallID == toolCallID {
			return true
		}
	}
	return false
}

// contextBoundaryEffect is the boundary a committed tool batch asks
// for, derived from a successful context tool result and the persisted
// arguments of the matching call.
type contextBoundaryEffect struct {
	tool     string
	followUp string
}

// contextBoundaryEffectFromStep finds a non-error result of a context
// tool in content and reads follow_up from the matching call in
// toolCalls. Error results and other tools yield no effect. A
// successful result without a parseable matching call is an error.
func contextBoundaryEffectFromStep(toolCalls []fantasy.ToolCallContent, content []fantasy.Content) (contextBoundaryEffect, bool, error) {
	for _, block := range content {
		result, ok := asToolResultContent(block)
		if !ok || !isContextToolName(result.ToolName) {
			continue
		}
		if isToolResultError(result) {
			continue
		}
		var call *fantasy.ToolCallContent
		for i := range toolCalls {
			if toolCalls[i].ToolCallID == result.ToolCallID {
				call = &toolCalls[i]
				break
			}
		}
		if call == nil {
			return contextBoundaryEffect{}, false, xerrors.Errorf("context tool result %q has no matching call", result.ToolCallID)
		}
		var args contextToolArgs
		if err := json.Unmarshal([]byte(call.Input), &args); err != nil {
			return contextBoundaryEffect{}, false, xerrors.Errorf("parse %s args: %w", result.ToolName, err)
		}
		return contextBoundaryEffect{tool: result.ToolName, followUp: args.FollowUp}, true, nil
	}
	return contextBoundaryEffect{}, false, nil
}

func isContextToolName(name string) bool {
	for _, candidate := range contextToolNames {
		if candidate == name {
			return true
		}
	}
	return false
}

// applyContextBoundaryEffect appends the boundary rows for effect to a
// tool step commit. For clear_context the order is: step rows, the
// chat_cleared triplet, post_tool_use hook rows, then the follow-up as
// the last row. Hook rows sit after the triplet so a model_context
// effect lands after the new prompt anchor instead of behind it.
func applyContextBoundaryEffect(
	messages stepMessagesForCommit,
	effect contextBoundaryEffect,
	hookResults []*chathooks.Result,
	modelConfigID uuid.UUID,
) (stepMessagesForCommit, error) {
	switch effect.tool {
	case clearContextToolName:
		triplet, err := buildClearMessages(buildClearMessagesInput{
			modelConfigID: modelConfigID,
			toolCallID:    "chat_cleared_" + uuid.NewString(),
			source:        chatloop.CompactionSourceAgent,
		})
		if err != nil {
			return stepMessagesForCommit{}, xerrors.Errorf("build clear messages: %w", err)
		}
		messages.Messages = append(messages.Messages, triplet...)
	default:
		return stepMessagesForCommit{}, xerrors.Errorf("unknown context tool %q", effect.tool)
	}
	messages, err := appendHookResultMessages(messages, hookResults, modelConfigID)
	if err != nil {
		return stepMessagesForCommit{}, err
	}
	followUp, err := modelOnlyUserRow(modelConfigID, effect.followUp)
	if err != nil {
		return stepMessagesForCommit{}, err
	}
	messages.Messages = append(messages.Messages, followUp)
	return messages, nil
}

// recordContextToolOutcomes counts every context tool result in
// results. deniedIDs lists the tool call ids whose results were
// synthesized by policy (exclusive batch, hook denial); other error
// results count as rejected.
func recordContextToolOutcomes(metrics *chatloop.Metrics, results []fantasy.ToolResultContent, deniedIDs map[string]bool) {
	for _, result := range results {
		if !isContextToolName(result.ToolName) {
			continue
		}
		outcome := chatloop.ContextToolOutcomeSuccess
		switch {
		case deniedIDs[result.ToolCallID]:
			outcome = chatloop.ContextToolOutcomeDenied
		case isToolResultError(result):
			outcome = chatloop.ContextToolOutcomeRejected
		}
		metrics.RecordContextToolCall(result.ToolName, outcome)
	}
}

// toolCallIDSet returns the set of tool call ids in results.
func toolCallIDSet(results []fantasy.ToolResultContent) map[string]bool {
	ids := make(map[string]bool, len(results))
	for _, result := range results {
		ids[result.ToolCallID] = true
	}
	return ids
}

func isToolResultError(result fantasy.ToolResultContent) bool {
	_, isError := result.Result.(fantasy.ToolResultOutputContentError)
	return isError
}
