package taskname

import (
	"context"
	"io"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const systemPrompt = `Generate a short workspace name from this AI task prompt.

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
- Respond with "task-workspace"
- Do not end with a random number`

func Generate(ctx context.Context, prompt, fallback string) (string, error) {
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

	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey == "" {
		return fallback, nil
	}

	anthropicClient := anthropic.NewClient(anthropic.DefaultClientOptions()...)

	stream, err := anthropicDataStream(ctx, anthropicClient, conversation)
	if err != nil {
		return fallback, xerrors.Errorf("create anthropic data stream: %w", err)
	}

	var acc aisdk.DataStreamAccumulator
	stream = stream.WithAccumulator(&acc)

	if err := stream.Pipe(io.Discard); err != nil {
		return fallback, xerrors.Errorf("pipe data stream")
	}

	if len(acc.Messages()) == 0 {
		return fallback, nil
	}

	generatedName := acc.Messages()[0].Content

	if err := codersdk.NameValid(generatedName); err != nil {
		return fallback, xerrors.Errorf("generated name %v not valid: %w", generatedName, err)
	}

	if generatedName == "task-workspace" {
		return fallback, nil
	}

	return generatedName, nil
}

func anthropicDataStream(ctx context.Context, client anthropic.Client, input []aisdk.Message) (aisdk.DataStream, error) {
	messages, system, err := aisdk.MessagesToAnthropic(input)
	if err != nil {
		return nil, xerrors.Errorf("convert messages to anthropic format: %w", err)
	}

	return aisdk.AnthropicToDataStream(client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_5HaikuLatest,
		MaxTokens: 24,
		System:    system,
		Messages:  messages,
	})), nil
}
