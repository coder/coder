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

/** Docs path explaining how Bedrock InvokeModel providers choose the model. */
export const bedrockInvokeModelDocsPath =
	"/ai-coder/ai-gateway/providers#how-invokemodel-selects-the-model";
