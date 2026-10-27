import type { AIBridgeProvider } from "#/api/typesGenerated";

const compactTokenFormatter = new Intl.NumberFormat("en-US", {
	notation: "compact",
	maximumFractionDigits: 1,
});

/** Formats a token count compactly, such as `22.5M`. */
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

/** Labels a configured provider with its display name, or its name if unset. */
export const getConfiguredProviderLabel = (provider: AIBridgeProvider) =>
	provider.display_name || provider.name;

/** Labels a configured provider name, or formats an unmatched name, such as
 * one recorded before configured names existed, as a provider type. */
export const getProviderNameLabel = (
	provider: string,
	providersByName: ReadonlyMap<string, AIBridgeProvider> | undefined,
) => {
	const configured = providersByName?.get(provider);
	return configured
		? getConfiguredProviderLabel(configured)
		: getProviderDisplayName(provider);
};

/** Indexes providers by name; an unloaded list yields an empty map. */
export const getProvidersByName = (
	providers: readonly AIBridgeProvider[] | undefined,
): ReadonlyMap<string, AIBridgeProvider> =>
	new Map((providers ?? []).map((provider) => [provider.name, provider]));
