const PROVIDER_STATUS_URLS: Record<string, string> = {
	anthropic: "https://status.anthropic.com",
};

export const getErrorTitle = (
	kind: string,
	mode: "retry" | "error",
): string => {
	switch (kind) {
		case "overloaded":
			return "Service overloaded";
		case "rate_limit":
			return "Rate limited";
		case "timeout":
			return "Request timeout";
		default:
			return mode === "retry" ? "Retrying request" : "Request failed";
	}
};

export const getProviderStatusURL = (
	kind: string,
	provider?: string,
): string | undefined => {
	if (!provider || kind !== "overloaded") {
		return undefined;
	}
	return PROVIDER_STATUS_URLS[provider.toLowerCase()];
};
