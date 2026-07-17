package chatprovider_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func TestAcceptsFilePartMediaType(t *testing.T) {
	t.Parallel()

	// A representative Responses-capable model ID. Non-Responses models
	// use the Chat Completions path, which accepts text/* natively.
	const responsesModel = "gpt-4o"
	const nonResponsesModel = "babbage-002"

	cases := []struct {
		name      string
		provider  string
		modelID   string
		mediaType string
		want      bool
	}{
		// OpenAI Responses accepts only images and PDFs.
		{"openai-json", "openai", responsesModel, "application/json", false},
		{"openai-text", "openai", responsesModel, "text/plain", false},
		{"openai-image", "openai", responsesModel, "image/png", true},
		{"openai-pdf", "openai", responsesModel, "application/pdf", true},

		// OpenAI Chat Completions (non-Responses models) accepts text/*
		// and audio as native file parts.
		{"openai-non-responses-text", "openai", nonResponsesModel, "text/plain", true},
		{"openai-non-responses-json", "openai", nonResponsesModel, "application/json", false},
		{"openai-non-responses-image", "openai", nonResponsesModel, "image/png", true},

		// Azure uses the Responses API, same as OpenAI.
		{"azure-text", "azure", responsesModel, "text/markdown", false},
		{"azure-image", "azure", responsesModel, "image/jpeg", true},

		// Anthropic accepts text/* as native documents, but not JSON.
		{"anthropic-text", "anthropic", "", "text/markdown", true},
		{"anthropic-json", "anthropic", "", "application/json", false},
		{"anthropic-pdf", "anthropic", "", "application/pdf", true},
		{"anthropic-image", "anthropic", "", "image/webp", true},

		// Bedrock wraps Anthropic, so it matches Anthropic.
		{"bedrock-text", "bedrock", "", "text/csv", true},
		{"bedrock-json", "bedrock", "", "application/json", false},

		// OpenAI-compatible accepts text/*, images, audio, and PDFs.
		{"openaicompat-text", "openai-compat", "", "text/plain", true},
		{"openaicompat-json", "openai-compat", "", "application/json", false},
		{"openaicompat-audio", "openai-compat", "", "audio/mpeg", true},

		// OpenRouter and Vercel do not accept text file parts.
		{"openrouter-text", "openrouter", "", "text/plain", false},
		{"openrouter-json", "openrouter", "", "application/json", false},
		{"openrouter-image", "openrouter", "", "image/png", true},
		{"vercel-text", "vercel", "", "text/plain", false},
		{"vercel-json", "vercel", "", "application/json", false},
		{"vercel-pdf", "vercel", "", "application/pdf", true},

		// Google passes all file parts through unfiltered.
		{"google-json", "google", "", "application/json", true},
		{"google-text", "google", "", "text/plain", true},
		{"google-anything", "google", "", "application/octet-stream", true},

		// Unknown providers reject everything so text-family content is
		// converted to text and still reaches the model.
		{"unknown-text", "made-up-provider", "", "text/plain", false},
		{"empty-text", "", "", "text/plain", false},

		// Base media type handling: parameters are stripped.
		{"anthropic-text-charset", "anthropic", "", "text/plain; charset=utf-8", true},
		{"openai-text-charset", "openai", responsesModel, "text/plain; charset=utf-8", false},

		// Provider name normalization is case-insensitive.
		{"anthropic-uppercase", "Anthropic", "", "text/plain", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.AcceptsFilePartMediaType(tc.provider, tc.modelID, tc.mediaType)
			if got != tc.want {
				t.Fatalf("AcceptsFilePartMediaType(%q, %q, %q) = %v, want %v",
					tc.provider, tc.modelID, tc.mediaType, got, tc.want)
			}
		})
	}
}
