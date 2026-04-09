package taskname

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/anthropic-sdk-go"
	anthropicoption "github.com/charmbracelet/anthropic-sdk-go/option"
	"github.com/charmbracelet/anthropic-sdk-go/packages/ssestream"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/coderd/util/namesgenerator"
	strutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultModel = anthropic.ModelClaudeHaiku4_5
	systemPrompt = `Generate a short task display name and name from this AI task prompt.
Identify the main task (the core action and subject) and base both names on it.
The task display name and name should be as similar as possible so a human can easily associate them.

Requirements for task display name (generate this first):
- Human-readable description
- Maximum 64 characters total
- Should concisely describe the main task

Requirements for task name:
- Should be derived from the display name
- Only lowercase letters, numbers, and hyphens
- No spaces or underscores
- Maximum 27 characters total
- Should concisely describe the main task

Output format (must be valid JSON):
{
	"display_name": "<display_name>",
	"task_name": "<task_name>"
}

Examples:
Prompt: "Help me debug a Python script" →
{
	"display_name": "Debug Python script",
	"task_name": "python-debug"
}

Prompt: "Create a React dashboard component" →
{
	"display_name": "React dashboard component",
	"task_name": "react-dashboard"
}

Prompt: "Analyze sales data from Q3" →
{
	"display_name": "Analyze Q3 sales data",
	"task_name": "analyze-q3-sales"
}

Prompt: "Set up CI/CD pipeline" →
{
	"display_name": "CI/CD pipeline setup",
	"task_name": "setup-cicd"
}

Prompt: "Work on https://github.com/coder/coder/issues/1234" →
{
	"display_name": "Work on coder/coder #1234",
	"task_name": "coder-1234"
}

Prompt: "Fix https://github.com/org/repo/pull/567" →
{
	"display_name": "Fix org/repo PR #567",
	"task_name": "repo-pr-567"
}

If a suitable name cannot be created, output exactly:
{
	"display_name": "Task Unnamed",
	"task_name": "task-unnamed"
}

Do not include any additional keys, explanations, or text outside the JSON.`
)

var (
	ErrNoAPIKey        = xerrors.New("no api key provided")
	ErrNoNameGenerated = xerrors.New("no task name generated")
)

// extractJSON strips optional markdown code fences (```json or
// ```) that LLMs sometimes wrap around JSON output, returning
// only the inner JSON string. Only well-formed fences with a
// newline after the opening backticks are stripped; malformed
// fences are left untouched so that json.Unmarshal fails
// cleanly and the caller can fall back to other strategies.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Only strip when there is a newline separating the
		// fence line from the body. Without one we cannot
		// reliably tell the fence from the content.
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
			s = strings.TrimSuffix(s, "```")
			s = strings.TrimSpace(s)
		}
	}
	return s
}

type TaskName struct {
	Name        string `json:"task_name"`
	DisplayName string `json:"display_name"`
}

func getAnthropicAPIKeyFromEnv() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}

func getAnthropicModelFromEnv() anthropic.Model {
	return anthropic.Model(os.Getenv("ANTHROPIC_MODEL"))
}

// generateSuffix generates a random hex string between `0000` and `ffff`.
func generateSuffix() string {
	numMin := 0x00000
	numMax := 0x10000
	//nolint:gosec // We don't need a cryptographically secure random number generator for generating a task name suffix.
	num := rand.IntN(numMax-numMin) + numMin

	return fmt.Sprintf("%04x", num)
}

// generateFallback generates a random task name when other methods fail.
// Uses Docker-style name generation with a collision-resistant suffix.
func generateFallback() TaskName {
	// We have a 32 character limit for the name.
	// We have a 5 character suffix `-ffff`.
	// This leaves us with 27 characters for the name.
	name := namesgenerator.NameWith("-")
	name = name[:min(len(name), 27)]
	name = strings.TrimSuffix(name, "-")

	taskName := fmt.Sprintf("%s-%s", name, generateSuffix())
	displayName := strings.ReplaceAll(name, "-", " ")
	if len(displayName) > 0 {
		displayName = strings.ToUpper(displayName[:1]) + displayName[1:]
	}

	return TaskName{
		Name:        taskName,
		DisplayName: displayName,
	}
}

// generateFromPrompt creates a task name directly from the prompt by sanitizing it.
// This is used as a fallback when Claude fails to generate a name.
func generateFromPrompt(prompt string) (TaskName, error) {
	// Normalize newlines and tabs to spaces
	prompt = regexp.MustCompile(`[\n\r\t]+`).ReplaceAllString(prompt, " ")

	// Truncate prompt to 27 chars with full words for task name generation
	truncatedForName := prompt
	if len(prompt) > 27 {
		truncatedForName = strutil.Truncate(prompt, 27, strutil.TruncateWithFullWords)
	}

	// Generate task name from truncated prompt
	name := strings.ToLower(truncatedForName)
	// Replace whitespace (\t \r \n and spaces) sequences with hyphens
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, "-")
	// Remove all characters except lowercase letters, numbers, and hyphens
	name = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(name, "")
	// Collapse multiple consecutive hyphens into a single hyphen
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
	// Remove leading and trailing hyphens
	name = strings.Trim(name, "-")

	if len(name) == 0 {
		return TaskName{}, ErrNoNameGenerated
	}

	taskName := fmt.Sprintf("%s-%s", name, generateSuffix())

	// Use the initial prompt as display name, truncated to 64 chars with full words
	displayName := strutil.Truncate(prompt, 64, strutil.TruncateWithFullWords, strutil.TruncateWithEllipsis)
	displayName = strings.TrimSpace(displayName)
	if len(displayName) == 0 {
		// Ensure display name is never empty
		displayName = strings.ReplaceAll(name, "-", " ")
	}
	displayName = strutil.Capitalize(displayName)

	return TaskName{
		Name:        taskName,
		DisplayName: displayName,
	}, nil
}

// generateFromAnthropic uses Claude (Anthropic) to generate semantic task and display names from a user prompt.
// It sends the prompt to Claude with a structured system prompt requesting JSON output containing both names.
// Returns an error if the API call fails, the response is invalid, or Claude returns an "unnamed" placeholder.
func generateFromAnthropic(ctx context.Context, prompt string, apiKey string, model anthropic.Model, opts ...anthropicoption.RequestOption) (TaskName, error) {
	anthropicModel := model
	if anthropicModel == "" {
		anthropicModel = defaultModel
	}
	if apiKey == "" {
		return TaskName{}, ErrNoAPIKey
	}

	conversation := []aisdk.Message{
		{
			Role: "system",
			Parts: []aisdk.Part{{
				Type: aisdk.PartTypeText,
				Text: systemPrompt,
			}},
		},
		{
			Role: "user",
			Parts: []aisdk.Part{{
				Type: aisdk.PartTypeText,
				Text: prompt,
			}},
		},
	}

	anthropicOptions := anthropic.DefaultClientOptions()
	anthropicOptions = append(anthropicOptions, anthropicoption.WithAPIKey(apiKey))
	anthropicOptions = append(anthropicOptions, opts...)
	anthropicClient := anthropic.NewClient(anthropicOptions...)

	stream, err := anthropicDataStream(ctx, anthropicClient, anthropicModel, conversation)
	if err != nil {
		return TaskName{}, xerrors.Errorf("create anthropic data stream: %w", err)
	}

	var acc aisdk.DataStreamAccumulator
	stream = stream.WithAccumulator(&acc)

	if err := stream.Pipe(io.Discard); err != nil {
		return TaskName{}, xerrors.Errorf("pipe data stream")
	}

	if len(acc.Messages()) == 0 {
		return TaskName{}, ErrNoNameGenerated
	}

	// Parse the JSON response. LLMs sometimes wrap JSON in
	// markdown code fences (```json ... ```), so we strip
	// those before unmarshalling.
	var taskNameResponse TaskName
	if err := json.Unmarshal([]byte(extractJSON(acc.Messages()[0].Content)), &taskNameResponse); err != nil {
		return TaskName{}, xerrors.Errorf("failed to parse anthropic response: %w", err)
	}

	taskNameResponse.Name = strings.TrimSpace(taskNameResponse.Name)
	taskNameResponse.DisplayName = strings.TrimSpace(taskNameResponse.DisplayName)

	if taskNameResponse.Name == "" || taskNameResponse.Name == "task-unnamed" {
		return TaskName{}, xerrors.Errorf("anthropic returned invalid task name: %q", taskNameResponse.Name)
	}

	if taskNameResponse.DisplayName == "" || taskNameResponse.DisplayName == "Task Unnamed" {
		return TaskName{}, xerrors.Errorf("anthropic returned invalid task display name: %q", taskNameResponse.DisplayName)
	}

	// We append a suffix to the end of the task name to reduce
	// the chance of collisions. We truncate the task name to
	// a maximum of 27 bytes, so that when we append the
	// 5 byte suffix (`-` and 4 byte hex slug), it should
	// remain within the 32 byte workspace name limit.
	name := taskNameResponse.Name[:min(len(taskNameResponse.Name), 27)]
	name = strings.TrimSuffix(name, "-")
	name = fmt.Sprintf("%s-%s", name, generateSuffix())
	if err := codersdk.NameValid(name); err != nil {
		return TaskName{}, xerrors.Errorf("generated name %v not valid: %w", name, err)
	}

	displayName := taskNameResponse.DisplayName
	displayName = strings.TrimSpace(displayName)
	if len(displayName) == 0 {
		// Ensure display name is never empty
		displayName = strings.ReplaceAll(taskNameResponse.Name, "-", " ")
	}
	displayName = strutil.Capitalize(displayName)

	return TaskName{
		Name:        name,
		DisplayName: displayName,
	}, nil
}

// Generate creates a task name and display name from a user prompt.
// It attempts multiple strategies in order of preference:
//  1. Use Claude (Anthropic) to generate semantic names from the prompt if an API key is available
//  2. Sanitize the prompt directly into a valid task name
//  3. Generate a random name as a final fallback
//
// A suffix is always appended to task names to reduce collision risk.
// This function always succeeds and returns a valid TaskName.
func Generate(ctx context.Context, logger slog.Logger, prompt string) TaskName {
	if anthropicAPIKey := getAnthropicAPIKeyFromEnv(); anthropicAPIKey != "" {
		taskName, err := generateFromAnthropic(ctx, prompt, anthropicAPIKey, getAnthropicModelFromEnv())
		if err == nil {
			return taskName
		}
		// Anthropic failed, fall through to next fallback
		logger.Error(ctx, "unable to generate task name and display name from Anthropic", slog.Error(err))
	}

	// Try generating from prompt
	taskName, err := generateFromPrompt(prompt)
	if err == nil {
		return taskName
	}
	logger.Warn(ctx, "unable to generate task name and display name from prompt", slog.Error(err))

	// Final fallback
	return generateFallback()
}

func anthropicDataStream(ctx context.Context, client anthropic.Client, model anthropic.Model, input []aisdk.Message) (aisdk.DataStream, error) {
	messages, system, err := messagesToAnthropic(input)
	if err != nil {
		return nil, xerrors.Errorf("convert messages to anthropic format: %w", err)
	}

	return anthropicToDataStream(client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 100,
		System:    system,
		Messages:  messages,
	})), nil
}

// messagesToAnthropic converts internal message format to
// Anthropic's API format. This is a simplified version that
// only handles text parts since taskname only sends system
// and user text messages.
func messagesToAnthropic(messages []aisdk.Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam, error) {
	var anthropicMessages []anthropic.MessageParam
	var systemBlocks []anthropic.TextBlockParam

	for _, message := range messages {
		switch message.Role {
		case "system":
			for _, part := range message.Parts {
				if part.Type == aisdk.PartTypeText && part.Text != "" {
					systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
						Text: part.Text,
					})
				}
			}
		case "user", "assistant":
			role := anthropic.MessageParamRoleUser
			if message.Role == "assistant" {
				role = anthropic.MessageParamRoleAssistant
			}
			var content []anthropic.ContentBlockParamUnion
			for _, part := range message.Parts {
				if part.Type == aisdk.PartTypeText && part.Text != "" {
					content = append(content, anthropic.ContentBlockParamUnion{
						OfText: &anthropic.TextBlockParam{Text: part.Text},
					})
				}
			}
			if len(content) > 0 {
				anthropicMessages = append(anthropicMessages, anthropic.MessageParam{
					Role:    role,
					Content: content,
				})
			}
		default:
			return nil, nil, xerrors.Errorf("unsupported message role: %s", message.Role)
		}
	}

	return anthropicMessages, systemBlocks, nil
}

// anthropicToDataStream converts an Anthropic SSE stream into
// an aisdk DataStream. This is a local port of the aisdk-go
// bridge function using charmbracelet/anthropic-sdk-go types
// (which resolve to coder/anthropic-sdk-go via go.mod replace).
func anthropicToDataStream(stream *ssestream.Stream[anthropic.MessageStreamEventUnion]) aisdk.DataStream {
	return func(yield func(aisdk.DataStreamPart, error) bool) {
		var lastChunk *anthropic.MessageStreamEventUnion
		var finalReason aisdk.FinishReason = aisdk.FinishReasonUnknown
		var finalUsage aisdk.Usage
		var currentToolCall struct {
			ID   string
			Args string
		}

		for stream.Next() {
			chunk := stream.Current()
			lastChunk = &chunk

			event := chunk.AsAny()
			switch event := event.(type) {
			case anthropic.MessageStartEvent:
				if !yield(aisdk.StartStepStreamPart{
					MessageID: event.Message.ID,
				}, nil) {
					return
				}

			case anthropic.ContentBlockDeltaEvent:
				switch delta := event.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					if !yield(aisdk.TextStreamPart{Content: delta.Text}, nil) {
						return
					}
				case anthropic.InputJSONDelta:
					currentToolCall.Args += delta.PartialJSON
					if !yield(aisdk.ToolCallDeltaStreamPart{
						ToolCallID:    currentToolCall.ID,
						ArgsTextDelta: delta.PartialJSON,
					}, nil) {
						return
					}
				case anthropic.ThinkingDelta:
					if !yield(aisdk.ReasoningStreamPart{Content: delta.Thinking}, nil) {
						return
					}
				}

			case anthropic.ContentBlockStartEvent:
				if block, ok := event.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
					currentToolCall.ID = block.ID
					currentToolCall.Args = ""

					if !yield(aisdk.ToolCallStartStreamPart{
						ToolCallID: block.ID,
						ToolName:   block.Name,
					}, nil) {
						return
					}
				}

			case anthropic.MessageDeltaEvent:
				if event.Usage.OutputTokens != 0 {
					tokens := event.Usage.OutputTokens
					finalUsage.CompletionTokens = &tokens
				}

				switch event.Delta.StopReason {
				case "tool_use":
					finalReason = aisdk.FinishReasonToolCalls
					currentToolCall = struct {
						ID   string
						Args string
					}{}
				case "end_turn", "stop_sequence":
					finalReason = aisdk.FinishReasonStop
				case "max_tokens":
					finalReason = aisdk.FinishReasonLength
				}

			case anthropic.MessageStopEvent:
				if finalReason == aisdk.FinishReasonUnknown {
					finalReason = aisdk.FinishReasonStop
				}

				if !yield(aisdk.FinishStepStreamPart{
					FinishReason: finalReason,
					Usage:        finalUsage,
					IsContinued:  false,
				}, nil) {
					return
				}

				if !yield(aisdk.FinishMessageStreamPart{
					FinishReason: finalReason,
					Usage:        finalUsage,
				}, nil) {
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			yield(nil, xerrors.Errorf("anthropic stream error: %w", err))
			return
		}

		if lastChunk == nil || lastChunk.Type != "message_stop" {
			if finalReason == aisdk.FinishReasonUnknown {
				finalReason = aisdk.FinishReasonError
			}

			yield(aisdk.FinishMessageStreamPart{
				FinishReason: finalReason,
				Usage:        finalUsage,
			}, nil)
		}
	}
}
