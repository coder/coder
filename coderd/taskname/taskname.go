package taskname

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultModel = anthropic.ModelClaude3_5HaikuLatest
	systemPrompt = `Generate a short workspace name from this AI task prompt.

Requirements:
- Only lowercase letters, numbers, and hyphens
- Start with "task-"
- End with a random number between 0-99
- Maximum 32 characters total
- Descriptive of the main task

Examples:
- "Help me debug a Python script" → "task-python-debug-12"
- "Create a React dashboard component" → "task-react-dashboard-93"
- "Analyze sales data from Q3" → "task-analyze-q3-sales-37"
- "Set up CI/CD pipeline" → "task-setup-cicd-44"

If you cannot create a suitable name:
- Respond with "task-unnamed"
- Do not end with a random number`
)

var (
	ErrNoAPIKey        = errors.New("no api key provided")
	ErrNoNameGenerated = errors.New("no task name generated")
)

type options struct {
	apiKey string
	model  anthropic.Model
}

type option func(o *options)

func WithAPIKey(apiKey string) option {
	return func(o *options) {
		o.apiKey = apiKey
	}
}

func WithModel(model anthropic.Model) option {
	return func(o *options) {
		o.model = model
	}
}

func GetAnthropicAPIKeyFromEnv() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}

func GetAnthropicModelFromEnv() anthropic.Model {
	return anthropic.Model(os.Getenv("ANTHROPIC_MODEL"))
}

func Generate(ctx context.Context, prompt string, opts ...option) (string, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	if o.model == "" {
		o.model = defaultModel
	}
	if o.apiKey == "" {
		return "", ErrNoAPIKey
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
	anthropicOptions = append(anthropicOptions, anthropicoption.WithAPIKey(o.apiKey))
	anthropicClient := anthropic.NewClient(anthropicOptions...)

	stream, err := anthropicDataStream(ctx, anthropicClient, o.model, conversation)
	if err != nil {
		return "", xerrors.Errorf("create anthropic data stream: %w", err)
	}

	var acc aisdk.DataStreamAccumulator
	stream = stream.WithAccumulator(&acc)

	if err := stream.Pipe(io.Discard); err != nil {
		return "", xerrors.Errorf("pipe data stream")
	}

	if len(acc.Messages()) == 0 {
		return "", ErrNoNameGenerated
	}

	generatedName := acc.Messages()[0].Content

	if err := codersdk.NameValid(generatedName); err != nil {
		return "", xerrors.Errorf("generated name %v not valid: %w", generatedName, err)
	}

	if generatedName == "task-unnamed" {
		return "", ErrNoNameGenerated
	}

	return generatedName, nil
}

func anthropicDataStream(ctx context.Context, client anthropic.Client, model anthropic.Model, input []aisdk.Message) (aisdk.DataStream, error) {
	messages, system, err := aisdk.MessagesToAnthropic(input)
	if err != nil {
		return nil, xerrors.Errorf("convert messages to anthropic format: %w", err)
	}

	return aisdk.AnthropicToDataStream(client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 24,
		System:    system,
		Messages:  messages,
	})), nil
}
