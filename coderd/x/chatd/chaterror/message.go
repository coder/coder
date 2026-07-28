package chaterror

import (
	"fmt"
	"strings"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
)

// terminalMessage produces the user-facing error description shown
// when retries are exhausted. HTTP status codes are carried in the
// classified payload's StatusCode field and rendered as a separate
// footer chip by the UI, so they are intentionally omitted here to
// avoid duplicating the same information in two places.
func terminalMessage(classified ClassifiedError) string {
	subject := providerSubject(classified.Provider)
	switch classified.Kind {
	case codersdk.ChatErrorKindOverloaded:
		return stringutil.Capitalize(fmt.Sprintf("%s is temporarily overloaded.", subject))

	case codersdk.ChatErrorKindRateLimit:
		return stringutil.Capitalize(fmt.Sprintf("%s is rate limiting requests.", subject))

	case codersdk.ChatErrorKindTimeout:
		if !classified.Retryable && classified.StatusCode == 0 {
			return "The request timed out before it completed."
		}
		return stringutil.Capitalize(fmt.Sprintf("%s is temporarily unavailable.", subject))

	case codersdk.ChatErrorKindStreamSilenceTimeout:
		return stringutil.Capitalize(fmt.Sprintf(
			"%s did not send response data in time.", subject,
		))

	case codersdk.ChatErrorKindUsageLimit:
		return stringutil.Capitalize(fmt.Sprintf(
			"The usage quota for %s has been exceeded."+
				" Check the billing and quota settings for the provider account.",
			subject,
		))

	case codersdk.ChatErrorKindAuth:
		return fmt.Sprintf(
			"Authentication with %s failed."+
				" Check the API key and permissions.",
			subject,
		)

	case codersdk.ChatErrorKindConfig:
		return stringutil.Capitalize(fmt.Sprintf(
			"%s rejected the model configuration."+
				" Check the selected model and provider settings.",
			subject,
		))

	case codersdk.ChatErrorKindMissingKey:
		return "This conversation was started with an API key that is no longer available." +
			" Send your message again to continue."
	case codersdk.ChatErrorKindProviderDisabled:
		displayName := providerDisplayName(classified.Provider)
		return fmt.Sprintf(
			"The %s provider has been disabled."+
				" Contact your Coder administrator.",
			displayName,
		)
	case codersdk.ChatErrorKindContentFilter:
		return ContentFilterMessage(classified.Provider, "")
	default:
		if !classified.Retryable && classified.StatusCode == 0 {
			return "The chat request failed unexpectedly."
		}
		return stringutil.Capitalize(fmt.Sprintf("%s returned an unexpected error.", subject))
	}
}

// retryMessage produces a clean factual description suitable for
// display alongside the retry countdown UI. It omits HTTP status
// codes (surfaced separately in the payload) and remediation
// guidance (not actionable while auto-retrying).
func retryMessage(classified ClassifiedError) string {
	if classified.Retryable && classified.Message != "" {
		return classified.Message
	}

	subject := providerSubject(classified.Provider)
	switch classified.Kind {
	case codersdk.ChatErrorKindOverloaded:
		return stringutil.Capitalize(fmt.Sprintf("%s is temporarily overloaded.", subject))
	case codersdk.ChatErrorKindRateLimit:
		return stringutil.Capitalize(fmt.Sprintf("%s is rate limiting requests.", subject))
	case codersdk.ChatErrorKindTimeout:
		return stringutil.Capitalize(fmt.Sprintf("%s is temporarily unavailable.", subject))
	case codersdk.ChatErrorKindStreamSilenceTimeout:
		return stringutil.Capitalize(fmt.Sprintf(
			"%s did not send response data in time.", subject,
		))
	case codersdk.ChatErrorKindAuth:
		return fmt.Sprintf(
			"Authentication with %s failed.", subject,
		)
	case codersdk.ChatErrorKindConfig:
		return stringutil.Capitalize(fmt.Sprintf(
			"%s rejected the model configuration.", subject,
		))
	case codersdk.ChatErrorKindMissingKey:
		return "The API key for this conversation is no longer available."
	case codersdk.ChatErrorKindProviderDisabled:
		displayName := providerDisplayName(classified.Provider)
		return fmt.Sprintf(
			"The %s provider has been disabled by an administrator.",
			displayName,
		)
	default:
		return stringutil.Capitalize(fmt.Sprintf(
			"%s returned an unexpected error.", subject,
		))
	}
}

// ContentFilterMessage is the user-facing message for a response blocked by
// the provider's content filter.
func ContentFilterMessage(provider, category string) string {
	subject := providerSubject(provider)
	if category = strings.TrimSpace(category); category != "" {
		return stringutil.Capitalize(fmt.Sprintf(
			"%s blocked this response under its content policy (%s).", subject, category,
		))
	}
	return stringutil.Capitalize(fmt.Sprintf(
		"%s blocked this response under its content policy.", subject,
	))
}

func providerSubject(provider string) string {
	if displayName := providerDisplayName(provider); displayName != "AI" && displayName != "" {
		return displayName
	}
	return "the AI provider"
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
		return "AI"
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
