package chaterror

import (
	"fmt"
	"strings"
)

func userFacingMessage(classified ClassifiedError) string {
	switch classified.Kind {
	case KindOverloaded:
		return overloadedMessage(classified.Provider, classified.StatusCode)
	case KindRateLimit:
		return rateLimitMessage(classified.Provider, classified.StatusCode)
	case KindTimeout:
		return timeoutMessage(classified)
	case KindStartupTimeout:
		return startupTimeoutMessage(classified.Provider)
	case KindAuth:
		return authMessage(classified.Provider)
	case KindConfig:
		return configMessage(classified.Provider)
	default:
		return genericMessage(classified)
	}
}

func overloadedMessage(provider string, statusCode int) string {
	subject := providerSubject(provider)
	if statusCode > 0 {
		return fmt.Sprintf(
			"%s is temporarily overloaded (HTTP %d). Please try again later.",
			subject,
			statusCode,
		)
	}
	return fmt.Sprintf("%s is temporarily overloaded. Please try again later.", subject)
}

func rateLimitMessage(provider string, statusCode int) string {
	subject := providerSubject(provider)
	if statusCode > 0 {
		return fmt.Sprintf(
			"%s is rate limiting requests (HTTP %d). Please try again later.",
			subject,
			statusCode,
		)
	}
	return fmt.Sprintf("%s is rate limiting requests. Please try again later.", subject)
}

func timeoutMessage(classified ClassifiedError) string {
	subject := providerSubject(classified.Provider)
	if classified.StatusCode > 0 {
		return fmt.Sprintf(
			"%s is temporarily unavailable (HTTP %d). Please try again later.",
			subject,
			classified.StatusCode,
		)
	}
	if classified.Retryable {
		return fmt.Sprintf("%s did not respond in time. Please try again.", subject)
	}
	return "The request timed out before it completed. Please try again."
}

func startupTimeoutMessage(provider string) string {
	return fmt.Sprintf(
		"%s did not start responding in time. Please try again.",
		providerSubject(provider),
	)
}

func authMessage(provider string) string {
	if displayName := providerDisplayName(provider); displayName != "" {
		return fmt.Sprintf(
			"Authentication with %s failed. Check the API key, permissions, and billing settings.",
			displayName,
		)
	}
	return "Authentication with the AI provider failed. Check the API key, permissions, and billing settings."
}

func configMessage(provider string) string {
	subject := providerSubject(provider)
	return fmt.Sprintf(
		"%s rejected the model configuration. Check the selected model and provider settings.",
		subject,
	)
}

func genericMessage(classified ClassifiedError) string {
	subject := providerSubject(classified.Provider)
	if classified.StatusCode > 0 {
		message := fmt.Sprintf(
			"%s returned an unexpected error (HTTP %d).",
			subject,
			classified.StatusCode,
		)
		if classified.Retryable {
			return message + " Please try again later."
		}
		return message + " Please try again."
	}
	if classified.Retryable {
		return fmt.Sprintf(
			"%s returned an unexpected error. Please try again later.",
			subject,
		)
	}
	return "The chat request failed unexpectedly. Please try again."
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
