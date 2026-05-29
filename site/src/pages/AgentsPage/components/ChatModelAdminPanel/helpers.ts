/**
 * Reads a value as a non-empty string, returning undefined for
 * empty strings, null, or undefined values.
 */
export function readOptionalString(value: unknown): string | undefined {
	if (typeof value !== "string") return undefined;
	const trimmed = value.trim();
	return trimmed || undefined;
}

export function normalizeProvider(provider: string): string {
	return provider.trim().toLowerCase();
}

const canonicalProviderBaseURLs: Record<string, string> = {
	anthropic: "https://api.anthropic.com",
	google: "https://generativelanguage.googleapis.com/v1beta",
	openai: "https://api.openai.com/v1",
	openrouter: "https://openrouter.ai/api/v1",
	vercel: "https://ai-gateway.vercel.sh/v1",
};

export function getDefaultProviderBaseURL(provider: string): string {
	return canonicalProviderBaseURLs[normalizeProvider(provider)] ?? "";
}

export function getProviderBaseURLPlaceholder(provider: string): string {
	switch (normalizeProvider(provider)) {
		case "azure":
			return "https://<resource-name>.openai.azure.com";
		case "bedrock":
			return "https://bedrock-runtime.<region>.amazonaws.com";
		case "openai-compat":
			return "https://api.example.com/v1";
		default:
			return getDefaultProviderBaseURL(provider) || "https://api.example.com";
	}
}
