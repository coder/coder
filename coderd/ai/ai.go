package ai

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kylecarbs/aisdk-go"
	"github.com/openai/openai-go"
	openaioption "github.com/openai/openai-go/option"
	"golang.org/x/xerrors"
	"google.golang.org/genai"

	"github.com/coder/coder/v2/codersdk"
)

type LanguageModel struct {
	codersdk.LanguageModel
	StreamFunc StreamFunc
}

type StreamOptions struct {
	SystemPrompt string
	Model        string
	Messages     []aisdk.Message
	Thinking     bool
	Tools        []aisdk.Tool
}

type StreamFunc func(ctx context.Context, options StreamOptions) (aisdk.DataStream, error)

// LanguageModels is a map of language model ID to language model.
type LanguageModels map[string]LanguageModel

func ModelsFromConfig(ctx context.Context, configs []codersdk.AIProviderConfig) (LanguageModels, error) {
	models := make(LanguageModels)

	for _, config := range configs {
		var streamFunc StreamFunc

		switch config.Type {
		case "openai":
			client := openai.NewClient(openaioption.WithAPIKey(config.APIKey))
			streamFunc = func(ctx context.Context, options StreamOptions) (aisdk.DataStream, error) {
				openaiMessages, err := aisdk.MessagesToOpenAI(options.Messages)
				if err != nil {
					return nil, err
				}
				tools := aisdk.ToolsToOpenAI(options.Tools)
				if options.SystemPrompt != "" {
					openaiMessages = append([]openai.ChatCompletionMessageParamUnion{
						openai.SystemMessage(options.SystemPrompt),
					}, openaiMessages...)
				}

				return aisdk.OpenAIToDataStream(client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
					Messages:  openaiMessages,
					Model:     options.Model,
					Tools:     tools,
					MaxTokens: openai.Int(8192),
				})), nil
			}
			if config.Models == nil {
				models, err := client.Models.List(ctx)
				if err != nil {
					return nil, err
				}
				config.Models = make([]string, len(models.Data))
				for i, model := range models.Data {
					config.Models[i] = model.ID
				}
			}
			break
		case "anthropic":
			client := anthropic.NewClient(anthropicoption.WithAPIKey(config.APIKey))
			streamFunc = func(ctx context.Context, options StreamOptions) (aisdk.DataStream, error) {
				anthropicMessages, systemMessage, err := aisdk.MessagesToAnthropic(options.Messages)
				if err != nil {
					return nil, err
				}
				if options.SystemPrompt != "" {
					systemMessage = []anthropic.TextBlockParam{
						*anthropic.NewTextBlock(options.SystemPrompt).OfRequestTextBlock,
					}
				}
				return aisdk.AnthropicToDataStream(client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
					Messages:  anthropicMessages,
					Model:     options.Model,
					System:    systemMessage,
					Tools:     aisdk.ToolsToAnthropic(options.Tools),
					MaxTokens: 8192,
				})), nil
			}
			if config.Models == nil {
				models, err := client.Models.List(ctx, anthropic.ModelListParams{})
				if err != nil {
					return nil, err
				}
				config.Models = make([]string, len(models.Data))
				for i, model := range models.Data {
					config.Models[i] = model.ID
				}
			}
			break
		case "google":
			client, err := genai.NewClient(ctx, &genai.ClientConfig{
				APIKey:  config.APIKey,
				Backend: genai.BackendGeminiAPI,
			})
			if err != nil {
				return nil, err
			}
			streamFunc = func(ctx context.Context, options StreamOptions) (aisdk.DataStream, error) {
				googleMessages, err := aisdk.MessagesToGoogle(options.Messages)
				if err != nil {
					return nil, err
				}
				tools, err := aisdk.ToolsToGoogle(options.Tools)
				if err != nil {
					return nil, err
				}
				var systemInstruction *genai.Content
				if options.SystemPrompt != "" {
					systemInstruction = &genai.Content{
						Parts: []*genai.Part{
							genai.NewPartFromText(options.SystemPrompt),
						},
						Role: "model",
					}
				}
				return aisdk.GoogleToDataStream(client.Models.GenerateContentStream(ctx, options.Model, googleMessages, &genai.GenerateContentConfig{
					SystemInstruction: systemInstruction,
					Tools:             tools,
				})), nil
			}
			if config.Models == nil {
				models, err := client.Models.List(ctx, &genai.ListModelsConfig{})
				if err != nil {
					return nil, err
				}
				config.Models = make([]string, len(models.Items))
				for i, model := range models.Items {
					config.Models[i] = model.Name
				}
			}
			break
		default:
			return nil, xerrors.Errorf("unsupported model type: %s", config.Type)
		}

		for _, model := range config.Models {
			models[model] = LanguageModel{
				LanguageModel: codersdk.LanguageModel{
					ID:          model,
					DisplayName: model,
					Provider:    config.Type,
				},
				StreamFunc: streamFunc,
			}
		}
	}

	return models, nil
}
