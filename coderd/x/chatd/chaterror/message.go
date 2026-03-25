package chaterror

import (
	"fmt"
	"strings"
)

func userFacingMessage(classified ClassifiedError) string {
	subject := providerSubject(classified.Provider)
	switch classified.Kind {
	case KindOverloaded:
		return optionalStatusMessage(
			subject,
			classified.StatusCode,
			"%s is temporarily overloaded (HTTP %d). Please try again later.",
			"%s is temporarily overloaded. Please try again later.",
		)
	case KindRateLimit:
		return optionalStatusMessage(
			subject,
			classified.StatusCode,
			"%s is rate limiting requests (HTTP %d). Please try again later.",
			"%s is rate limiting requests. Please try again later.",
		)
	case KindTimeout:
		if classified.StatusCode > 0 {
			return fmt.Sprintf(
				"%s is temporarily unavailable (HTTP %d). Please try again later.",
				subject,
				classified.StatusCode,
			)
		}
		if classified.Retryable {
			return fmt.Sprintf("%s is temporarily unavailable. Please try again later.", subject)
		}
		return "The request timed out before it completed. Please try again."
	case KindStartupTimeout:
		return fmt.Sprintf("%s did not start responding in time. Please try again.", subject)
	case KindAuth:
		if displayName := providerDisplayName(classified.Provider); displayName != "" {
			return fmt.Sprintf(
				"Authentication with %s failed. Check the API key, permissions, and billing settings.",
				displayName,
			)
		}
		return "Authentication with the AI provider failed. Check the API key, permissions, and billing settings."
	case KindConfig:
		return fmt.Sprintf(
			"%s rejected the model configuration. Check the selected model and provider settings.",
			subject,
		)
	default:
		if classified.StatusCode > 0 {
			suffix := " Please try again."
			if classified.Retryable {
				suffix = " Please try again later."
			}
			return fmt.Sprintf(
				"%s returned an unexpected error (HTTP %d).%s",
				subject,
				classified.StatusCode,
				suffix,
			)
		}
		if classified.Retryable {
			return fmt.Sprintf(
				"%s returned an unexpected error. Please try again later.",
				subject,
			)
		}
		return "The chat request failed unexpectedly. Please try again."
	}
}

func optionalStatusMessage(subject string, statusCode int, withStatus string, withoutStatus string) string {
	if statusCode > 0 {
		return fmt.Sprintf(withStatus, subject, statusCode)
	}
	return fmt.Sprintf(withoutStatus, subject)
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
