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

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	strutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultModel = anthropic.ModelClaude3_5HaikuLatest
	systemPrompt = `Generate a short task name and display name from this AI task prompt.
Identify the main task (the core action and subject) and base both names on it.
The task name and display name should be as similar as possible so a human can easily associate them.

Requirements for task name:
- Only lowercase letters, numbers, and hyphens
- No spaces or underscores
- Maximum 27 characters total
- Should concisely describe the main task

Requirements for display name:
- Human-readable description
- Maximum 64 characters total
- Should concisely describe the main task

Output format (must be valid JSON):
{
  "task_name": "<task_name>",
  "display_name": "<display_name>"
}

Examples:
Prompt: "Help me debug a Python script" →
{
  "task_name": "python-debug",
  "display_name": "Debug Python script"
}

Prompt: "Create a React dashboard component" →
{
  "task_name": "react-dashboard",
  "display_name": "React dashboard component"
}

Prompt: "Analyze sales data from Q3" →
{
  "task_name": "analyze-q3-sales",
  "display_name": "Analyze Q3 sales data"
}

Prompt: "Set up CI/CD pipeline" →
{
  "task_name": "setup-cicd",
  "display_name": "CI/CD pipeline setup"
}

If a suitable name cannot be created, output exactly:
{
  "task_name": "task-unnamed",
  "display_name": "Task Unnamed"
}

Do not include any additional keys, explanations, or text outside the JSON.`
)

var (
	ErrNoAPIKey        = xerrors.New("no api key provided")
	ErrNoNameGenerated = xerrors.New("no task name generated")
)

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
	//
	// Unfortunately, `namesgenerator.GetRandomName(0)` will
	// generate names that are longer than 27 characters, so
	// we just trim these down to length.
	name := strings.ReplaceAll(namesgenerator.GetRandomName(0), "_", "-")
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
	name = strings.ReplaceAll(name, " ", "-")
	name = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
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
	displayName = strings.ToUpper(displayName[:1]) + displayName[1:]

	return TaskName{
		Name:        taskName,
		DisplayName: displayName,
	}, nil
}

// generateFromAnthropic uses Claude (Anthropic) to generate semantic task and display names from a user prompt.
// It sends the prompt to Claude with a structured system prompt requesting JSON output containing both names.
// Returns an error if the API call fails, the response is invalid, or Claude returns an "unnamed" placeholder.
func generateFromAnthropic(ctx context.Context, prompt string, apiKey string, model anthropic.Model) (TaskName, error) {
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

	// Parse the JSON response
	var taskNameResponse TaskName
	if err := json.Unmarshal([]byte(acc.Messages()[0].Content), &taskNameResponse); err != nil {
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
	displayName = strings.ToUpper(displayName[:1]) + displayName[1:]

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
func Generate(ctx context.Context, prompt string) TaskName {
	if anthropicAPIKey := getAnthropicAPIKeyFromEnv(); anthropicAPIKey != "" {
		taskName, err := generateFromAnthropic(ctx, prompt, anthropicAPIKey, getAnthropicModelFromEnv())
		if err == nil {
			return taskName
		}
		// Anthropic failed, fall through to next fallback
	}

	// Try generating from prompt
	taskName, err := generateFromPrompt(prompt)
	if err == nil {
		return taskName
	}

	// Final fallback
	return generateFallback()
}

func anthropicDataStream(ctx context.Context, client anthropic.Client, model anthropic.Model, input []aisdk.Message) (aisdk.DataStream, error) {
	messages, system, err := aisdk.MessagesToAnthropic(input)
	if err != nil {
		return nil, xerrors.Errorf("convert messages to anthropic format: %w", err)
	}

	return aisdk.AnthropicToDataStream(client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 50,
		System:    system,
		Messages:  messages,
	})), nil
}
