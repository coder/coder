export const formatProviderLabel = (provider: string): string => {
	const normalized = provider.trim().toLowerCase();
	switch (normalized) {
		case "openai":
			return "OpenAI";
		case "anthropic":
			return "Anthropic";
		case "azure":
			return "Azure OpenAI";
		case "bedrock":
			return "AWS Bedrock";
		case "claude-platform-aws":
			return "Claude Platform for AWS";
		case "google":
			return "Google";
		case "openai-compat":
		case "openai-compatible":
		case "openai_compatible":
			return "OpenAI-compatible";
		case "openrouter":
			return "OpenRouter";
		case "vercel":
			return "Vercel AI Gateway";
		default:
			if (!normalized) {
				return "Unknown";
			}
			return `${normalized[0].toUpperCase()}${normalized.slice(1)}`;
	}
};
