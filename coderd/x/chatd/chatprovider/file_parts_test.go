package chatprovider_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func TestAcceptsFilePartMediaType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		provider  string
		mediaType string
		want      bool
	}{
		// OpenAI Responses accepts only images and PDFs.
		{"openai-json", "openai", "application/json", false},
		{"openai-text", "openai", "text/plain", false},
		{"openai-image", "openai", "image/png", true},
		{"openai-pdf", "openai", "application/pdf", true},

		// Azure uses the Responses API, same as OpenAI.
		{"azure-text", "azure", "text/markdown", false},
		{"azure-image", "azure", "image/jpeg", true},

		// Anthropic accepts text/* as native documents, but not JSON.
		{"anthropic-text", "anthropic", "text/markdown", true},
		{"anthropic-json", "anthropic", "application/json", false},
		{"anthropic-pdf", "anthropic", "application/pdf", true},
		{"anthropic-image", "anthropic", "image/webp", true},

		// Bedrock wraps Anthropic, so it matches Anthropic.
		{"bedrock-text", "bedrock", "text/csv", true},
		{"bedrock-json", "bedrock", "application/json", false},

		// OpenAI-compatible accepts text/*, images, audio, and PDFs.
		{"openaicompat-text", "openai-compat", "text/plain", true},
		{"openaicompat-json", "openai-compat", "application/json", false},
		{"openaicompat-audio", "openai-compat", "audio/mpeg", true},

		// OpenRouter and Vercel do not accept text file parts.
		{"openrouter-text", "openrouter", "text/plain", false},
		{"openrouter-image", "openrouter", "image/png", true},
		{"vercel-text", "vercel", "text/plain", false},
		{"vercel-pdf", "vercel", "application/pdf", true},

		// Google passes all file parts through unfiltered.
		{"google-json", "google", "application/json", true},
		{"google-text", "google", "text/plain", true},
		{"google-anything", "google", "application/octet-stream", true},

		// Unknown providers reject everything so text-ish content is
		// converted to text and still reaches the model.
		{"unknown-text", "made-up-provider", "text/plain", false},
		{"empty-text", "", "text/plain", false},

		// Base media type handling: parameters are stripped.
		{"anthropic-text-charset", "anthropic", "text/plain; charset=utf-8", true},
		{"openai-text-charset", "openai", "text/plain; charset=utf-8", false},

		// Provider name normalization is case-insensitive.
		{"anthropic-uppercase", "Anthropic", "text/plain", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.AcceptsFilePartMediaType(tc.provider, tc.mediaType)
			if got != tc.want {
				t.Fatalf("AcceptsFilePartMediaType(%q, %q) = %v, want %v",
					tc.provider, tc.mediaType, got, tc.want)
			}
		})
	}
}
