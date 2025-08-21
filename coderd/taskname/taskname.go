package taskname

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/moby/moby/pkg/namesgenerator"
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
- Maximum 28 characters total
- Descriptive of the main task

Examples:
- "Help me debug a Python script" → "task-python-debug"
- "Create a React dashboard component" → "task-react-dashboard"
- "Analyze sales data from Q3" → "task-analyze-q3-sales"
- "Set up CI/CD pipeline" → "task-setup-cicd"

If you cannot create a suitable name:
- Respond with "task-unnamed"`
)

var (
	ErrNoAPIKey        = xerrors.New("no api key provided")
	ErrNoNameGenerated = xerrors.New("no task name generated")
)

type options struct {
	apiKey string
	model  anthropic.Model
}

type Option func(o *options)

func WithAPIKey(apiKey string) Option {
	return func(o *options) {
		o.apiKey = apiKey
	}
}

func WithModel(model anthropic.Model) Option {
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

// generateSuffix generates a random hex string between `100` and `fff`.
func generateSuffix() string {
	numMin := 0x100
	numMax := 0x1000
	//nolint:gosec // We don't need a cryptographically secure random number generator for generating a task name suffix.
	num := rand.IntN(numMax-numMin) + numMin

	return fmt.Sprintf("%x", num)
}

func GenerateFallback() string {
	// We have a 32 character limit for the name.
	// We have a 5 character prefix `task-`.
	// We have a 4 character suffix `-fff`.
	// This leaves us with 23 characters for the middle.
	//
	// Unfortunately, `namesgenerator.GetRandomName(0)` will
	// generate names that are longer than 23 characters, so
	// we just trim these down to length.
	name := strings.ReplaceAll(namesgenerator.GetRandomName(0), "_", "-")
	name = name[:min(len(name), 23)]
	name = strings.TrimSuffix(name, "-")

	return fmt.Sprintf("task-%s-%s", name, generateSuffix())
}

func Generate(ctx context.Context, prompt string, opts ...Option) (string, error) {
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

	return fmt.Sprintf("%s-%s", generatedName, generateSuffix()), nil
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
