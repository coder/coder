import type { AIProvider, AIProviderType } from "#/api/typesGenerated";

export const AI_PROVIDER_SETTINGS_TYPE_BEDROCK = "bedrock";

type SettingsWire = {
	readonly _type?: string;
};

export const isBedrockAIProvider = (
	provider: Pick<AIProvider, "type" | "settings">,
): boolean => {
	if (provider.type === "bedrock") {
		return true;
	}
	if (provider.type !== "anthropic") {
		return false;
	}
	const settings = provider.settings as SettingsWire | null | undefined;
	return (
		settings != null && settings._type === AI_PROVIDER_SETTINGS_TYPE_BEDROCK
	);
};

export const canonicalAIProviderType = (
	provider: Pick<AIProvider, "type" | "settings">,
): AIProviderType =>
	isBedrockAIProvider(provider) ? "bedrock" : provider.type;

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
