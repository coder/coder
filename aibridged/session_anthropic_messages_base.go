package aibridged

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

type AnthropicMessagesSessionBase struct {
	id string

	req *BetaMessageNewParamsWrapper

	baseURL, key string
	logger       slog.Logger

	tracker Tracker
	toolMgr ToolManager
}

func (s *AnthropicMessagesSessionBase) Init(logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) string {
	s.id = uuid.NewString()

	s.logger = logger.With(slog.F("session_id", s.id))

	s.baseURL = baseURL
	s.key = key

	s.tracker = tracker
	s.toolMgr = toolMgr

	return s.id
}

func (s *AnthropicMessagesSessionBase) LastUserPrompt() (*string, error) {
	if s.req == nil {
		return nil, xerrors.New("nil request")
	}

	return s.req.LastUserPrompt()
}

func (s *AnthropicMessagesSessionBase) Model() Model {
	var model string
	if s.req == nil {
		model = "?"
	} else {
		model = string(s.req.Model)
	}

	return Model{
		Provider:  "anthropic",
		ModelName: model,
	}
}

func (s *AnthropicMessagesSessionBase) injectTools() {
	if s.req == nil {
		return
	}

	// Inject tools.
	for _, tool := range s.toolMgr.ListTools() {
		s.req.Tools = append(s.req.Tools, anthropic.BetaToolUnionParam{
			OfTool: &anthropic.BetaToolParam{
				InputSchema: anthropic.BetaToolInputSchemaParam{
					Properties: tool.Params,
					Required:   tool.Required,
				},
				Name:        tool.ID,
				Description: anthropic.String(tool.Description),
				Type:        anthropic.BetaToolTypeCustom,
			},
		})
	}

	// Note: Parallel tool calls are disabled to avoid tool_use/tool_result block mismatches.
	s.req.ToolChoice = anthropic.BetaToolChoiceUnionParam{
		OfAny: &anthropic.BetaToolChoiceAnyParam{
			Type:                   "auto",
			DisableParallelToolUse: anthropic.Bool(true),
		},
	}
}

// isSmallFastModel checks if the model is a small/fast model (Haiku 3.5).
// These models are optimized for tasks like code autocomplete and other small, quick operations.
// See `ANTHROPIC_SMALL_FAST_MODEL`: https://docs.anthropic.com/en/docs/claude-code/settings#environment-variables
func (s *AnthropicMessagesSessionBase) isSmallFastModel() bool {
	return strings.Contains(string(s.req.Model), "3-5-haiku")
}
