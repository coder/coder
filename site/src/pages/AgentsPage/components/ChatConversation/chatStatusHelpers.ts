import type { ChatProviderFailureKind } from "../../utils/usageLimitMessage";
import { normalizeProvider } from "../ChatModelAdminPanel/helpers";

const PROVIDER_STATUS_URLS: Record<string, string> = {
	anthropic: "https://status.anthropic.com",
};

/**
 * Resolves aliases for provider names used in status lookups.
 * Falls back to the base normalization for unknown providers.
 */
const resolveProviderAlias = (provider?: string): string | undefined => {
	if (!provider) {
		return undefined;
	}
	const normalized = normalizeProvider(provider);
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

export const getProviderStatusURL = (
	kind: ChatProviderFailureKind | (string & {}),
	provider?: string,
): string | undefined => {
	if (kind !== "overloaded") {
		return undefined;
	}
	const resolved = resolveProviderAlias(provider);
	return resolved ? PROVIDER_STATUS_URLS[resolved] : undefined;
};
