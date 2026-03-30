import type { ChatProviderFailureKind } from "../../utils/usageLimitMessage";

const PROVIDER_STATUS_URLS: Record<string, string> = {
	anthropic: "https://status.anthropic.com",
};

const normalizeProvider = (provider?: string): string | undefined => {
	const normalized = provider?.trim().toLowerCase();
	if (!normalized) {
		return undefined;
	}

	switch (normalized) {
		case "azure openai":
		case "azure-openai":
			return "azure";
		case "openai compat":
		case "openai compatible":
		case "openai_compat":
			return "openai-compat";
		default:
			return normalized;
	}
};

const humanizeKind = (kind: string): string => {
	const words = kind
		.trim()
		.split(/[_\-\s]+/)
		.filter(Boolean);
	if (words.length === 0) {
		return "Unexpected error";
	}
	return words
		.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
		.join(" ");
};

export const getErrorTitle = (
	kind: ChatProviderFailureKind | (string & {}),
	mode: "retry" | "error",
): string => {
	switch (kind) {
		case "overloaded":
			return "Service overloaded";
		case "rate_limit":
			return "Rate limited";
		case "timeout":
			return "Request timed out";
		case "startup_timeout":
			return "Startup timed out";
		case "auth":
			return "Authentication failed";
		case "config":
			return "Configuration error";
		default:
			return mode === "retry" ? "Retrying request" : "Request failed";
	}
};

export const getKindLabel = (
	kind: ChatProviderFailureKind | (string & {}),
): string => {
	switch (kind) {
		case "generic":
			return "Unexpected error";
		case "overloaded":
			return "Overloaded";
		case "rate_limit":
			return "Rate limit";
		case "timeout":
			return "Timeout";
		case "startup_timeout":
			return "Startup timeout";
		case "auth":
			return "Authentication";
		case "config":
			return "Configuration";
		default:
			return humanizeKind(kind);
	}
};

export const getProviderStatusURL = (
	kind: ChatProviderFailureKind | (string & {}),
	provider?: string,
): string | undefined => {
	if (kind !== "overloaded") {
		return undefined;
	}
	const normalized = normalizeProvider(provider);
	return normalized ? PROVIDER_STATUS_URLS[normalized] : undefined;
};
