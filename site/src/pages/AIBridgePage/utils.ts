const compactTokenFormatter = new Intl.NumberFormat("en-US", {
	notation: "compact",
	maximumFractionDigits: 1,
});

/**
 * Formats a token count compactly for badges, such as `950`, `111.7K`, or
 * `22.5M`. Badges pair this with a tooltip that shows the exact count.
 */
export const roundTokenDisplay = (tokens: number) =>
	compactTokenFormatter.format(tokens);

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
