package chatd

import (
	"context"
	"encoding/json"
	"maps"
	"strings"

	"charm.land/fantasy"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// workflowReportToolName is the server tool that a workflow step child
	// calls with its result. The name is reserved: the dynamic tool parsers
	// skip it, so chatd runs it instead of the client.
	workflowReportToolName = "workflow_report"
	// workflowRunLabel marks a child chat as a workflow step. The value is
	// the run ID.
	workflowRunLabel = "coder_workflow_run"

	workflowReportDescription = "Report the result of your task to the workflow that started you. " +
		"Call this tool one time, when your work is done. The input must match the schema. " +
		"The workflow reads only this value. It does not read your final message."
	workflowReminder = "Call workflow_report now with a value that matches its schema. Do no other work."
)

// workflowReportTool validates its input against the step schema on the
// server. A valid value ends the turn through StopAfterTools. An invalid
// value returns an error result, and the model can try again in the same
// turn.
type workflowReportTool struct {
	info     fantasy.ToolInfo
	resolved *jsonschema.Resolved
	opts     fantasy.ProviderOptions
}

var _ fantasy.AgentTool = (*workflowReportTool)(nil)

func newWorkflowReportTool(raw json.RawMessage) (*workflowReportTool, error) {
	resolved, err := checkStepSchema(raw)
	if err != nil {
		return nil, err
	}
	var shape struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return nil, xerrors.Errorf("parse report schema shape: %w", err)
	}
	return &workflowReportTool{
		info: fantasy.ToolInfo{
			Name:        workflowReportToolName,
			Description: workflowReportDescription,
			Parameters:  shape.Properties,
			Required:    shape.Required,
		},
		resolved: resolved,
	}, nil
}

func (t *workflowReportTool) Info() fantasy.ToolInfo { return t.info }

func (t *workflowReportTool) ProviderOptions() fantasy.ProviderOptions { return t.opts }

func (t *workflowReportTool) SetProviderOptions(opts fantasy.ProviderOptions) { t.opts = opts }

func (t *workflowReportTool) Run(_ context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var value any
	if err := json.Unmarshal([]byte(call.Input), &value); err != nil {
		return fantasy.NewTextErrorResponse("The input is not valid JSON: " + err.Error()), nil
	}
	if err := t.resolved.Validate(value); err != nil {
		return fantasy.NewTextErrorResponse("The value does not match the schema: " + err.Error() +
			". Call workflow_report again with a corrected value."), nil
	}
	return fantasy.NewTextResponse("Value accepted. The workflow has your result."), nil
}

// checkStepSchema accepts a JSON Schema whose top level is an object. The
// $schema keyword must be empty, draft-07 or draft 2020-12, because the
// validator supports only those.
func checkStepSchema(raw json.RawMessage) (*jsonschema.Resolved, error) {
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, xerrors.Errorf("parse step schema: %w", err)
	}
	if s.Type != "object" {
		return nil, xerrors.New(`the step schema must have "type": "object" at the top level`)
	}
	switch strings.TrimSpace(s.Schema) {
	case "",
		"http://json-schema.org/draft-07/schema#",
		"https://json-schema.org/draft-07/schema#",
		"https://json-schema.org/draft/2020-12/schema":
	default:
		return nil, xerrors.Errorf("unsupported $schema %q: use draft-07 or 2020-12", s.Schema)
	}
	resolved, err := s.Resolve(nil)
	if err != nil {
		return nil, xerrors.Errorf("resolve step schema: %w", err)
	}
	return resolved, nil
}

// workflowStepTools returns the dynamic tools column for a step child. It
// holds only the workflow_report definition with the step schema.
func workflowStepTools(schema json.RawMessage) (json.RawMessage, error) {
	raw, err := json.Marshal([]codersdk.DynamicTool{{
		Name:        workflowReportToolName,
		Description: workflowReportDescription,
		InputSchema: schema,
	}})
	if err != nil {
		return nil, xerrors.Errorf("marshal step tools: %w", err)
	}
	return raw, nil
}

// appendWorkflowReportTool adds workflow_report to the tools of a workflow
// step child. Other chats keep their tools.
func appendWorkflowReportTool(tools []fantasy.AgentTool, chat database.Chat) ([]fantasy.AgentTool, error) {
	if !chat.ParentChatID.Valid || !chat.DynamicTools.Valid {
		return tools, nil
	}
	var defs []codersdk.DynamicTool
	if err := json.Unmarshal(chat.DynamicTools.RawMessage, &defs); err != nil {
		// appendDynamicTools reports a broken column. A broken column is
		// not a workflow step.
		return tools, nil //nolint:nilerr // see above
	}
	for _, def := range defs {
		if def.Name != workflowReportToolName {
			continue
		}
		tool, err := newWorkflowReportTool(def.InputSchema)
		if err != nil {
			// startStep checked the schema before it created the chat.
			return nil, xerrors.Errorf("build workflow_report for chat %s: %w", chat.ID, err)
		}
		return append(tools, tool), nil
	}
	return tools, nil
}

// withWorkflowReportStop ends the turn after workflow_report when the turn
// has the tool.
func withWorkflowReportStop(stop map[string]struct{}, tools []fantasy.AgentTool) map[string]struct{} {
	for _, tool := range tools {
		if tool.Info().Name != workflowReportToolName {
			continue
		}
		out := make(map[string]struct{}, len(stop)+1)
		maps.Copy(out, stop)
		out[workflowReportToolName] = struct{}{}
		return out
	}
	return stop
}

// workflowStepReport reads a step child's history. value is the input of
// the last workflow_report call with a successful result. reminded is true
// when the child already got the reminder.
func workflowStepReport(
	ctx context.Context, db database.Store, chatID uuid.UUID,
) (value json.RawMessage, reminded bool, err error) {
	msgs, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	if err != nil {
		return nil, false, xerrors.Errorf("get step messages: %w", err)
	}
	calls := map[string]json.RawMessage{}
	for _, msg := range msgs {
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			continue
		}
		for _, part := range parts {
			switch {
			case msg.Role == database.ChatMessageRoleUser &&
				part.Type == codersdk.ChatMessagePartTypeText &&
				strings.TrimSpace(part.Text) == workflowReminder:
				reminded = true
			case part.Type == codersdk.ChatMessagePartTypeToolCall &&
				part.ToolName == workflowReportToolName:
				calls[part.ToolCallID] = part.Args
			case part.Type == codersdk.ChatMessagePartTypeToolResult && !part.IsError:
				if args, ok := calls[part.ToolCallID]; ok {
					value = args // a later accepted call wins
				}
			}
		}
	}
	return value, reminded, nil
}
