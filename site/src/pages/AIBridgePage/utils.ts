export const roundTokenDisplay = (tokens: number) => {
	if (tokens >= 1000) {
		return `${(tokens / 1000).toFixed(1)}k`;
	}
	return tokens.toString();
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
