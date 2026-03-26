package chaterror

import (
	"fmt"
	"strings"
)

// buildMessage produces the user-facing error description for a
// classified error. Terminal errors (forRetry=false) include HTTP
// status codes and actionable guidance; retry messages
// (forRetry=true) are clean factual statements suitable for display
// alongside the retry countdown UI.
func buildMessage(classified ClassifiedError, forRetry bool) string {
	subject := providerSubject(classified.Provider)
	switch classified.Kind {
	case KindOverloaded:
		if !forRetry && classified.StatusCode > 0 {
			return fmt.Sprintf(
				"%s is temporarily overloaded (HTTP %d).",
				subject, classified.StatusCode,
			)
		}
		return fmt.Sprintf("%s is temporarily overloaded.", subject)

	case KindRateLimit:
		if !forRetry && classified.StatusCode > 0 {
			return fmt.Sprintf(
				"%s is rate limiting requests (HTTP %d).",
				subject, classified.StatusCode,
			)
		}
		return fmt.Sprintf("%s is rate limiting requests.", subject)

	case KindTimeout:
		if !forRetry && classified.StatusCode > 0 {
			return fmt.Sprintf(
				"%s is temporarily unavailable (HTTP %d).",
				subject, classified.StatusCode,
			)
		}
		if !forRetry && !classified.Retryable {
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
		base := fmt.Sprintf("Authentication with %s failed.", displayName)
		if forRetry {
			return base
		}
		return base + " Check the API key, permissions, and billing settings."

	case KindConfig:
		base := fmt.Sprintf(
			"%s rejected the model configuration.", subject,
		)
		if forRetry {
			return base
		}
		return base + " Check the selected model and provider settings."

	default:
		if !forRetry && classified.StatusCode > 0 {
			return fmt.Sprintf(
				"%s returned an unexpected error (HTTP %d).",
				subject, classified.StatusCode,
			)
		}
		if !forRetry && !classified.Retryable {
			return "The chat request failed unexpectedly."
		}
		return fmt.Sprintf("%s returned an unexpected error.", subject)
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
