import { formatCostMicros, MICROS_PER_DOLLAR } from "#/utils/currency";

const compactTokenFormatter = new Intl.NumberFormat("en-US", {
	notation: "compact",
	maximumFractionDigits: 1,
});

/** Formats a token count compactly, such as `22.5M`. */
export const roundTokenDisplay = (tokens: number) =>
	compactTokenFormatter.format(tokens);

const CENT_MICROS = MICROS_PER_DOLLAR / 100;

/** Formats a session cost, collapsing amounts under a cent to `<$0.01`. */
export const formatSessionCost = (micros: number) =>
	micros > 0 && micros < CENT_MICROS ? "<$0.01" : formatCostMicros(micros);

const PROMPT_PREVIEW_MAX_CHARS = 500;

/** Cuts a prompt to a short preview for the sessions list. */
export const truncatePromptPreview = (prompt: string) => {
	if (prompt.length <= PROMPT_PREVIEW_MAX_CHARS) {
		return prompt;
	}
	// Slice by code point to avoid splitting emoji.
	const codePoints = Array.from(prompt);
	if (codePoints.length <= PROMPT_PREVIEW_MAX_CHARS) {
		return prompt;
	}
	return `${codePoints.slice(0, PROMPT_PREVIEW_MAX_CHARS).join("").trimEnd()}…`;
};

export const roundDurationDisplay = (duration: number) => {
	if (duration >= 1000) {
		return `${(duration / 1000).toFixed(1)}s`;
	}
	return `${duration.toFixed(0)}ms`;
};

export const getProviderDisplayName = (provider: string) => {
	switch (provider) {
		case "anthropic":
			return "Anthropic";
		case "openai":
			return "OpenAI";
		case "google":
			return "Google";
		case "azure":
			return "Azure OpenAI";
		case "bedrock":
			return "AWS Bedrock";
		case "copilot":
			return "GitHub Copilot";
		case "openai-compat":
			return "OpenAI-compatible";
		case "openrouter":
			return "OpenRouter";
		case "vercel":
			return "Vercel";
		default: {
			if (!provider) {
				return "Unknown";
			}
			return provider.charAt(0).toUpperCase() + provider.slice(1);
		}
	}
};
