package chaterror

import (
	"fmt"
	"strings"
)

// terminalMessage produces the user-facing error description shown
// when retries are exhausted. HTTP status codes are carried in the
// classified payload's StatusCode field and rendered as a separate
// footer chip by the UI, so they are intentionally omitted here to
// avoid duplicating the same information in two places.
func terminalMessage(classified ClassifiedError) string {
	subject := providerSubject(classified.Provider)
	switch classified.Kind {
	case KindOverloaded:
		return fmt.Sprintf("%s is temporarily overloaded.", subject)

	case KindRateLimit:
		return fmt.Sprintf("%s is rate limiting requests.", subject)

	case KindTimeout:
		if !classified.Retryable && classified.StatusCode == 0 {
			return "The request timed out before it completed."
		}
		return fmt.Sprintf("%s is temporarily unavailable.", subject)

	case KindStartupTimeout:
		return fmt.Sprintf(
			"%s did not start responding in time.", subject,
		)

	case KindAuth:
		displayName := providerDisplayName(classified.Provider)
		if displayName == "" {
			displayName = "the AI provider"
		}
		return fmt.Sprintf(
			"Authentication with %s failed."+
				" Check the API key, permissions, and billing settings.",
			displayName,
		)

	case KindConfig:
		return fmt.Sprintf(
			"%s rejected the model configuration."+
				" Check the selected model and provider settings.",
			subject,
		)

	default:
		if !classified.Retryable && classified.StatusCode == 0 {
			return "The chat request failed unexpectedly."
		}
		return fmt.Sprintf("%s returned an unexpected error.", subject)
	}
}

// retryMessage produces a clean factual description suitable for
// display alongside the retry countdown UI. It omits HTTP status
// codes (surfaced separately in the payload) and remediation
// guidance (not actionable while auto-retrying).
func retryMessage(classified ClassifiedError) string {
	subject := providerSubject(classified.Provider)
	switch classified.Kind {
	case KindOverloaded:
		return fmt.Sprintf("%s is temporarily overloaded.", subject)
	case KindRateLimit:
		return fmt.Sprintf("%s is rate limiting requests.", subject)
	case KindTimeout:
		return fmt.Sprintf("%s is temporarily unavailable.", subject)
	case KindStartupTimeout:
		return fmt.Sprintf(
			"%s did not start responding in time.", subject,
		)
	case KindAuth:
		displayName := providerDisplayName(classified.Provider)
		if displayName == "" {
			displayName = "the AI provider"
		}
		return fmt.Sprintf(
			"Authentication with %s failed.", displayName,
		)
	case KindConfig:
		return fmt.Sprintf(
			"%s rejected the model configuration.", subject,
		)
	default:
		return fmt.Sprintf(
			"%s returned an unexpected error.", subject,
		)
	}
}

func providerSubject(provider string) string {
	if displayName := providerDisplayName(provider); displayName != "" {
		return displayName
	}
	return "The AI provider"
}

func providerDisplayName(provider string) string {
	switch normalizeProvider(provider) {
	case "anthropic":
		return "Anthropic"
	case "azure":
		return "Azure OpenAI"
	case "bedrock":
		return "AWS Bedrock"
	case "google":
		return "Google"
	case "openai":
		return "OpenAI"
	case "openai-compat":
		return "OpenAI Compatible"
	case "openrouter":
		return "OpenRouter"
	case "vercel":
		return "Vercel AI Gateway"
	default:
		return ""
	}
}

func normalizeProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "azure openai", "azure-openai":
		return "azure"
	case "openai compat", "openai compatible", "openai_compat":
		return "openai-compat"
	default:
		return normalized
	}
}
