package chatexec

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

const maxExactIntFloat64 = 1 << 53

func buildLocalTools(
	defs []agentsdk.ChatRunnerToolDefinition,
	getLocalConn func(context.Context) (workspacesdk.AgentConn, error),
) ([]fantasy.AgentTool, error) {
	if len(defs) == 0 {
		return nil, nil
	}

	result := make([]fantasy.AgentTool, 0, len(defs))
	for i, def := range defs {
		tool, ok, err := localToolFromDefinition(def, getLocalConn)
		if err != nil {
			return nil, xerrors.Errorf("builtin tool %d (%q): %w", i, def.Name, err)
		}
		if !ok {
			continue
		}
		result = append(result, tool)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func localToolFromDefinition(
	def agentsdk.ChatRunnerToolDefinition,
	getLocalConn func(context.Context) (workspacesdk.AgentConn, error),
) (fantasy.AgentTool, bool, error) {
	if def.Name == "" {
		return nil, false, xerrors.New("name is required")
	}

	requireLocalConn := func() error {
		if getLocalConn != nil {
			return nil
		}
		return xerrors.New("workspace connection resolver is not configured")
	}

	switch def.Name {
	case "execute":
		if err := requireLocalConn(); err != nil {
			return nil, false, err
		}
		return chattool.Execute(chattool.ExecuteOptions{GetWorkspaceConn: getLocalConn}), true, nil
	case "read_file":
		if err := requireLocalConn(); err != nil {
			return nil, false, err
		}
		return chattool.ReadFile(chattool.ReadFileOptions{GetWorkspaceConn: getLocalConn}), true, nil
	case "write_file":
		if err := requireLocalConn(); err != nil {
			return nil, false, err
		}
		return chattool.WriteFile(chattool.WriteFileOptions{GetWorkspaceConn: getLocalConn}), true, nil
	case "edit_files":
		if err := requireLocalConn(); err != nil {
			return nil, false, err
		}
		return chattool.EditFiles(chattool.EditFilesOptions{GetWorkspaceConn: getLocalConn}), true, nil
	case "process_output":
		if err := requireLocalConn(); err != nil {
			return nil, false, err
		}
		return chattool.ProcessOutput(chattool.ProcessToolOptions{GetWorkspaceConn: getLocalConn}), true, nil
	case "process_list":
		if err := requireLocalConn(); err != nil {
			return nil, false, err
		}
		return chattool.ProcessList(chattool.ProcessToolOptions{GetWorkspaceConn: getLocalConn}), true, nil
	case "process_signal":
		if err := requireLocalConn(); err != nil {
			return nil, false, err
		}
		return chattool.ProcessSignal(chattool.ProcessToolOptions{GetWorkspaceConn: getLocalConn}), true, nil
	default:
		// Runtime context includes more built-ins than the agent can execute
		// locally today. Ignore unsupported definitions instead of advertising
		// tools that would fail at runtime.
		return nil, false, nil
	}
}

// buildControlPlaneTools creates tools that call back to the control
// plane through the agent-authenticated chat-runner API.
func buildControlPlaneTools(
	defs []agentsdk.ChatRunnerToolDefinition,
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) ([]fantasy.AgentTool, error) {
	if len(defs) == 0 {
		return nil, nil
	}

	result := make([]fantasy.AgentTool, 0, len(defs))
	for _, def := range defs {
		tool, ok := controlPlaneToolFromDefinition(def, client, chatID, leaseEpoch)
		if ok {
			result = append(result, tool)
		}
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func controlPlaneToolFromDefinition(
	def agentsdk.ChatRunnerToolDefinition,
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) (fantasy.AgentTool, bool) {
	switch def.Name {
	case "list_templates":
		return buildListTemplatesTool(client, chatID, leaseEpoch), true
	case "read_template":
		return buildReadTemplateTool(client, chatID, leaseEpoch), true
	default:
		return nil, false
	}
}

func buildMCPTools(
	mcpTools []agentsdk.ChatRunnerMCPTool,
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) []fantasy.AgentTool {
	if len(mcpTools) == 0 {
		return nil
	}

	result := make([]fantasy.AgentTool, 0, len(mcpTools))
	for _, tool := range mcpTools {
		if strings.TrimSpace(tool.ToolName) == "" {
			continue
		}
		result = append(result, buildMCPTool(tool, client, chatID, leaseEpoch))
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func buildMCPTool(
	tool agentsdk.ChatRunnerMCPTool,
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) fantasy.AgentTool {
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if len(tool.InputSchema) > 0 {
		_ = json.Unmarshal(tool.InputSchema, &schema)
	}

	base := fantasy.NewAgentTool(
		tool.ToolName,
		tool.Description,
		func(ctx context.Context, input json.RawMessage, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			resp, err := client.ChatRunnerMCPToolCall(ctx, agentsdk.ChatRunnerMCPToolCallRequest{
				ChatID:            chatID,
				LeaseEpoch:        leaseEpoch,
				MCPServerConfigID: tool.MCPServerConfigID,
				ToolName:          tool.ToolName,
				Args:              normalizeMCPToolArgs(input),
			})
			if err != nil {
				return fantasy.ToolResponse{}, err
			}

			result := fantasy.NewTextResponse(textFromRawJSON(resp.Result))
			result.IsError = resp.IsError
			return result, nil
		},
	)

	return &mcpToolWrapper{
		base:        base,
		name:        tool.ToolName,
		description: tool.Description,
		parameters:  schema.Properties,
		required:    schema.Required,
	}
}

type mcpToolWrapper struct {
	base        fantasy.AgentTool
	name        string
	description string
	parameters  map[string]any
	required    []string
}

func (t *mcpToolWrapper) Info() fantasy.ToolInfo {
	required := t.required
	if required == nil {
		required = []string{}
	}
	return fantasy.ToolInfo{
		Name:        t.name,
		Description: t.description,
		Parameters:  t.parameters,
		Required:    required,
		Parallel:    true,
	}
}

func (t *mcpToolWrapper) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(call.Input) == "" {
		call.Input = "{}"
	}
	return t.base.Run(ctx, call)
}

func (t *mcpToolWrapper) ProviderOptions() fantasy.ProviderOptions {
	return t.base.ProviderOptions()
}

func (t *mcpToolWrapper) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.base.SetProviderOptions(opts)
}

func normalizeMCPToolArgs(input json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(append([]byte(nil), trimmed...))
}

func textFromRawJSON(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text
	}
	return string(trimmed)
}

const (
	listTemplatesToolDescription = "List available workspace templates. Optionally filter by a " +
		"search query matching template name or description. " +
		"Use this to find a template before creating a workspace. " +
		"Results are ordered by number of active developers (most popular first). " +
		"Returns 10 per page. Use the page parameter to paginate through results."
	readTemplateToolDescription = "Get details about a workspace template, including its " +
		"configurable parameters. Use this after finding a " +
		"template with list_templates and before creating a " +
		"workspace with create_workspace."
)

type listTemplatesArgs struct {
	Query string `json:"query,omitempty"`
	Page  int    `json:"page,omitempty"`
}

type readTemplateArgs struct {
	TemplateID string `json:"template_id"`
}

type templateSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
}

type listTemplatesToolResponse struct {
	Templates  []templateSummary `json:"templates"`
	Count      int               `json:"count"`
	Page       int               `json:"page"`
	TotalPages int               `json:"total_pages"`
	TotalCount int               `json:"total_count"`
}

type templateOption struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value"`
}

type templateParameter struct {
	Name        string           `json:"name"`
	Type        string           `json:"type"`
	Required    bool             `json:"required"`
	DisplayName string           `json:"display_name,omitempty"`
	Description string           `json:"description,omitempty"`
	Default     string           `json:"default,omitempty"`
	Mutable     bool             `json:"mutable,omitempty"`
	Options     []templateOption `json:"options,omitempty"`
}

type readTemplateToolResponse struct {
	Template   templateSummary     `json:"template"`
	Parameters []templateParameter `json:"parameters"`
}

func buildListTemplatesTool(
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"list_templates",
		listTemplatesToolDescription,
		func(ctx context.Context, args listTemplatesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			resp, err := client.ChatRunnerListTemplates(ctx, agentsdk.ChatRunnerListTemplatesRequest{
				ChatID:     chatID,
				LeaseEpoch: leaseEpoch,
				Query:      args.Query,
				Page:       args.Page,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			templates := make([]templateSummary, 0, len(resp.Templates))
			for _, template := range resp.Templates {
				item := templateSummary{
					ID:   template.ID.String(),
					Name: template.Name,
				}
				if display := strings.TrimSpace(template.DisplayName); display != "" {
					item.DisplayName = display
				}
				if desc := strings.TrimSpace(template.Description); desc != "" {
					item.Description = truncateRunes(desc, 200)
				}
				templates = append(templates, item)
			}

			totalPages := 1
			if resp.PageSize > 0 {
				totalPages = (resp.TotalCount + resp.PageSize - 1) / resp.PageSize
			}

			return toolResponse(listTemplatesToolResponse{
				Templates:  templates,
				Count:      len(templates),
				Page:       resp.Page,
				TotalPages: totalPages,
				TotalCount: resp.TotalCount,
			}), nil
		},
	)
}

func buildReadTemplateTool(
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_template",
		readTemplateToolDescription,
		func(ctx context.Context, args readTemplateArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			templateIDStr := strings.TrimSpace(args.TemplateID)
			if templateIDStr == "" {
				return fantasy.NewTextErrorResponse("template_id is required"), nil
			}
			templateID, err := uuid.Parse(templateIDStr)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("invalid template_id: %w", err).Error(),
				), nil
			}

			resp, err := client.ChatRunnerReadTemplate(ctx, agentsdk.ChatRunnerReadTemplateRequest{
				ChatID:     chatID,
				LeaseEpoch: leaseEpoch,
				TemplateID: templateID,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			result := readTemplateToolResponse{
				Template: templateSummary{
					ID:   resp.Template.ID.String(),
					Name: resp.Template.Name,
				},
				Parameters: make([]templateParameter, 0, len(resp.Parameters)),
			}
			if display := strings.TrimSpace(resp.Template.DisplayName); display != "" {
				result.Template.DisplayName = display
			}
			if desc := strings.TrimSpace(resp.Template.Description); desc != "" {
				result.Template.Description = desc
			}

			for _, detail := range resp.Parameters {
				param := templateParameter{
					Name:     detail.Name,
					Type:     detail.Type,
					Required: detail.Required,
				}
				if display := strings.TrimSpace(detail.DisplayName); display != "" {
					param.DisplayName = display
				}
				if desc := strings.TrimSpace(detail.Description); desc != "" {
					param.Description = truncateRunes(desc, 300)
				}
				if detail.DefaultValue != "" {
					param.Default = detail.DefaultValue
				}
				if detail.Mutable {
					param.Mutable = true
				}
				if len(detail.Options) > 0 {
					param.Options = make([]templateOption, 0, len(detail.Options))
					for _, option := range detail.Options {
						item := templateOption{
							Name:  option.Name,
							Value: option.Value,
						}
						if desc := strings.TrimSpace(option.Description); desc != "" {
							item.Description = desc
						}
						param.Options = append(param.Options, item)
					}
				}
				result.Parameters = append(result.Parameters, param)
			}

			return toolResponse(result), nil
		},
	)
}

func truncateRunes(value string, maxLen int) string {
	if maxLen <= 0 || value == "" {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen])
}

// toolResponse marshals a JSON-serializable tool result into a fantasy
// response. Tool results are always JSON objects so the frontend can
// safely parse them.
func toolResponse(result any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
}

func buildDynamicTools(
	logger slog.Logger,
	defs []agentsdk.ChatRunnerToolDefinition,
) []fantasy.AgentTool {
	if len(defs) == 0 {
		return nil
	}

	result := make([]fantasy.AgentTool, 0, len(defs))
	for _, def := range defs {
		tool := &dynamicTool{
			name:        def.Name,
			description: def.Description,
		}
		if len(def.InputSchema) > 0 {
			var schema struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			}
			if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
				logger.Warn(context.Background(), "failed to parse dynamic tool input schema",
					slog.F("tool_name", def.Name),
					slog.Error(err),
				)
			} else {
				tool.parameters = schema.Properties
				tool.required = schema.Required
			}
		}
		if len(def.ProviderConfig) > 0 {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(def.ProviderConfig, &raw); err != nil {
				logger.Warn(context.Background(), "failed to parse dynamic tool provider config",
					slog.F("tool_name", def.Name),
					slog.Error(err),
				)
			} else {
				opts, err := fantasy.UnmarshalProviderOptions(raw)
				if err != nil {
					logger.Warn(context.Background(), "failed to decode dynamic tool provider config",
						slog.F("tool_name", def.Name),
						slog.Error(err),
					)
				} else {
					tool.opts = opts
				}
			}
		}
		result = append(result, tool)
	}
	return result
}

type dynamicTool struct {
	name        string
	description string
	parameters  map[string]any
	required    []string
	opts        fantasy.ProviderOptions
}

func (t *dynamicTool) Name() string {
	return t.name
}

func (t *dynamicTool) Description() string {
	return t.description
}

func (t *dynamicTool) Parameters() map[string]any {
	return t.parameters
}

func (t *dynamicTool) Required() []string {
	return t.required
}

func (t *dynamicTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
		Required:    t.Required(),
	}
}

func (*dynamicTool) Run(_ context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextErrorResponse(
		"dynamic tool called in chatloop — this is a bug; dynamic tools should be handled by the client",
	), nil
}

func (t *dynamicTool) ProviderOptions() fantasy.ProviderOptions {
	return t.opts
}

func (t *dynamicTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.opts = opts
}

func buildProviderTools(
	defs []agentsdk.ChatRunnerToolDefinition,
	getLocalConn func(context.Context) (workspacesdk.AgentConn, error),
	logger slog.Logger,
) ([]chatloop.ProviderTool, error) {
	if len(defs) == 0 {
		return nil, nil
	}

	result := make([]chatloop.ProviderTool, 0, len(defs))
	for i, def := range defs {
		tool, err := providerToolFromDefinition(def, getLocalConn, logger)
		if err != nil {
			return nil, xerrors.Errorf("provider tool %d (%q): %w", i, def.Name, err)
		}
		result = append(result, tool)
	}
	return result, nil
}

type providerDefinedToolConfig struct {
	ID   string         `json:"id"`
	Args map[string]any `json:"args"`
}

func providerToolFromDefinition(
	def agentsdk.ChatRunnerToolDefinition,
	getLocalConn func(context.Context) (workspacesdk.AgentConn, error),
	logger slog.Logger,
) (chatloop.ProviderTool, error) {
	if def.Name == "" {
		return chatloop.ProviderTool{}, xerrors.New("name is required")
	}
	if len(def.ProviderConfig) == 0 {
		return chatloop.ProviderTool{}, xerrors.New("provider_config is required")
	}

	var cfg providerDefinedToolConfig
	decoder := json.NewDecoder(bytes.NewReader(def.ProviderConfig))
	decoder.UseNumber()
	if err := decoder.Decode(&cfg); err != nil {
		return chatloop.ProviderTool{}, xerrors.Errorf("decode provider_config: %w", err)
	}
	if cfg.ID == "" {
		return chatloop.ProviderTool{}, xerrors.New("provider_config.id is required")
	}

	tool := chatloop.ProviderTool{
		Definition: fantasy.ProviderDefinedTool{
			ID:   cfg.ID,
			Name: def.Name,
			Args: cfg.Args,
		},
	}
	if def.Name != "computer" {
		return tool, nil
	}
	if getLocalConn == nil {
		return chatloop.ProviderTool{}, xerrors.New("workspace connection resolver is not configured")
	}

	width, err := requiredPositiveInt(cfg.Args, "display_width_px")
	if err != nil {
		return chatloop.ProviderTool{}, err
	}
	height, err := requiredPositiveInt(cfg.Args, "display_height_px")
	if err != nil {
		return chatloop.ProviderTool{}, err
	}
	tool.Runner = chattool.NewComputerUseTool(width, height, getLocalConn, nil, quartz.NewReal(), logger.Named("computer_use"))
	return tool, nil
}

func requiredPositiveInt(args map[string]any, key string) (int, error) {
	if args == nil {
		return 0, xerrors.Errorf("provider_config.args.%s is required", key)
	}
	value, ok := args[key]
	if !ok {
		return 0, xerrors.Errorf("provider_config.args.%s is required", key)
	}
	parsed, ok := exactIntFromAny(value)
	if !ok {
		return 0, xerrors.Errorf("provider_config.args.%s must be an integer", key)
	}
	if parsed <= 0 {
		return 0, xerrors.Errorf("provider_config.args.%s must be positive", key)
	}
	maxInt := int64(^uint(0) >> 1)
	if parsed > maxInt {
		return 0, xerrors.Errorf("provider_config.args.%s exceeds int range", key)
	}
	return int(parsed), nil
}

func exactIntFromAny(value any) (int64, bool) {
	const maxInt64 = int64(^uint64(0) >> 1)

	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		asUint64 := uint64(typed)
		if asUint64 > uint64(maxInt64) {
			return 0, false
		}
		return int64(asUint64), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(maxInt64) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		f := float64(typed)
		if math.Trunc(f) != f || math.IsNaN(f) || math.IsInf(f, 0) || f < -maxExactIntFloat64 || f > maxExactIntFloat64 {
			return 0, false
		}
		return int64(f), true
	case float64:
		if math.Trunc(typed) != typed || math.IsNaN(typed) || math.IsInf(typed, 0) || typed < -maxExactIntFloat64 || typed > maxExactIntFloat64 {
			return 0, false
		}
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
