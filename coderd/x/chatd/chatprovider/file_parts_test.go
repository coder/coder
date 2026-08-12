package chatprovider_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestAcceptsFilePartMediaType(t *testing.T) {
	t.Parallel()

	// A representative Responses-capable model ID. Non-Responses models
	// use the Chat Completions path, which accepts text/* natively.
	const responsesModel = "gpt-4o"
	const nonResponsesModel = "babbage-002"

	forceResponses := true
	forceCompletions := false

	cases := []struct {
		name      string
		provider  string
		modelID   string
		mediaType string
		want      bool
		override  *bool
	}{
		// OpenAI Responses accepts only images and PDFs.
		{"openai-json", "openai", responsesModel, "application/json", false, nil},
		{"openai-text", "openai", responsesModel, "text/plain", false, nil},
		{"openai-image", "openai", responsesModel, "image/png", true, nil},
		{"openai-pdf", "openai", responsesModel, "application/pdf", true, nil},

		// OpenAI Chat Completions (non-Responses models) accepts text/*
		// and audio as native file parts.
		{"openai-non-responses-text", "openai", nonResponsesModel, "text/plain", true, nil},
		{"openai-non-responses-json", "openai", nonResponsesModel, "application/json", false, nil},
		{"openai-non-responses-image", "openai", nonResponsesModel, "image/png", true, nil},

		// Azure uses the Responses API, same as OpenAI.
		{"azure-text", "azure", responsesModel, "text/markdown", false, nil},
		{"azure-image", "azure", responsesModel, "image/jpeg", true, nil},

		// Anthropic accepts text/* as native documents, but not JSON.
		{"anthropic-text", "anthropic", "", "text/markdown", true, nil},
		{"anthropic-json", "anthropic", "", "application/json", false, nil},
		{"anthropic-pdf", "anthropic", "", "application/pdf", true, nil},
		{"anthropic-image", "anthropic", "", "image/webp", true, nil},

		// Bedrock wraps Anthropic, so it matches Anthropic.
		{"bedrock-text", "bedrock", "", "text/csv", true, nil},
		{"bedrock-json", "bedrock", "", "application/json", false, nil},

		// OpenAI-compatible accepts text/*, images, audio, and PDFs.
		{"openaicompat-text", "openai-compat", "", "text/plain", true, nil},
		{"openaicompat-json", "openai-compat", "", "application/json", false, nil},
		{"openaicompat-audio", "openai-compat", "", "audio/mpeg", true, nil},

		// OpenRouter and Vercel do not accept text file parts.
		{"openrouter-text", "openrouter", "", "text/plain", false, nil},
		{"openrouter-json", "openrouter", "", "application/json", false, nil},
		{"openrouter-image", "openrouter", "", "image/png", true, nil},
		{"vercel-text", "vercel", "", "text/plain", false, nil},
		{"vercel-json", "vercel", "", "application/json", false, nil},
		{"vercel-pdf", "vercel", "", "application/pdf", true, nil},

		// Google passes all file parts through unfiltered.
		{"google-json", "google", "", "application/json", true, nil},
		{"google-text", "google", "", "text/plain", true, nil},
		{"google-anything", "google", "", "application/octet-stream", true, nil},

		// Unknown providers reject everything so text-family content is
		// converted to text and still reaches the model.
		{"unknown-text", "made-up-provider", "", "text/plain", false, nil},
		{"empty-text", "", "", "text/plain", false, nil},

		// Base media type handling: parameters are stripped.
		{"anthropic-text-charset", "anthropic", "", "text/plain; charset=utf-8", true, nil},
		{"openai-text-charset", "openai", responsesModel, "text/plain; charset=utf-8", false, nil},

		// Provider name normalization is case-insensitive.
		{"anthropic-uppercase", "Anthropic", "", "text/plain", true, nil},

		{"openai-forced-responses-text", "openai", nonResponsesModel, "text/plain", false, &forceResponses},
		{"openai-forced-responses-image", "openai", nonResponsesModel, "image/png", true, &forceResponses},
		{"openai-forced-completions-text", "openai", responsesModel, "text/plain", true, &forceCompletions},
		{"openai-forced-completions-audio", "openai", responsesModel, "audio/mpeg", true, &forceCompletions},

		// Azure has no equivalent provider option, so it keeps using the
		// known-model list even when a config sets the override.
		{"azure-ignores-forced-completions", "azure", responsesModel, "text/plain", false, &forceCompletions},
		{"azure-ignores-forced-responses", "azure", nonResponsesModel, "text/plain", true, &forceResponses},

		{"anthropic-ignores-override", "anthropic", "", "text/plain", true, &forceResponses},
		{"openaicompat-ignores-override", "openai-compat", "", "text/plain", true, &forceResponses},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var openAIConfig *codersdk.ChatModelOpenAIConfig
			if tc.override != nil {
				openAIConfig = &codersdk.ChatModelOpenAIConfig{UseResponsesAPI: tc.override}
			}
			model := chatprovider.NewModel(
				&chattest.FakeModel{ProviderName: tc.provider, ModelName: tc.modelID},
				openAIConfig,
			)
			got := model.AcceptsFilePartMediaType(tc.mediaType)
			if got != tc.want {
				t.Fatalf("AcceptsFilePartMediaType(%q, %q, %q, %v) = %v, want %v",
					tc.provider, tc.modelID, tc.mediaType, tc.override, got, tc.want)
			}
		})
	}
}
