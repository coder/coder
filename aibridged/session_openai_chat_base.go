package aibridged

import (
	"github.com/openai/openai-go"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

type OpenAIChatSessionBase struct {
	id string

	req *ChatCompletionNewParamsWrapper

	baseURL, key string
	logger       slog.Logger

	tracker Tracker
	toolMgr ToolManager
}

func (s *OpenAIChatSessionBase) Init(id string, logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) {
	s.id = id

	s.logger = logger.With(slog.F("session_id", s.id))

	s.baseURL = baseURL
	s.key = key

	s.tracker = tracker
	s.toolMgr = toolMgr
}

func (s *OpenAIChatSessionBase) LastUserPrompt() (*string, error) {
	if s.req == nil {
		return nil, xerrors.New("nil request")
	}

	return s.req.LastUserPrompt()
}

func (s *OpenAIChatSessionBase) Model() Model {
	var model string
	if s.req == nil {
		model = "?"
	} else {
		model = s.req.Model
	}

	return Model{
		Provider:  "openai",
		ModelName: model,
	}
}

func (s *OpenAIChatSessionBase) newErrorResponse(err error) map[string]interface{} {
	return map[string]interface{}{
		"error":   true,
		"message": err.Error(),
	}
}

func (s *OpenAIChatSessionBase) injectTools() {
	if s.req == nil {
		return
	}

	// Inject tools.
	for _, tool := range s.toolMgr.ListTools() {
		fn := openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        tool.ID,
				Strict:      openai.Bool(false), // TODO: configurable.
				Description: openai.String(tool.Description),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": tool.Params,
					// "additionalProperties": false, // Only relevant when strict=true.
				},
			},
		}

		// Otherwise the request fails with "None is not of type 'array'" if a nil slice is given.
		if len(tool.Required) > 0 {
			// Must list ALL properties when strict=true.
			fn.Function.Parameters["required"] = tool.Required
		}

		s.req.Tools = append(s.req.Tools, fn)
	}
}
