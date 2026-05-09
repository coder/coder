import type * as TypesGen from "#/api/typesGenerated";
import { isImageRelatedError } from "./chatError";

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

export const getErrorTitle = (
	kind: TypesGen.ChatErrorKind,
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
		case "usage_limit":
			return "Usage limit reached";
		default:
			return mode === "retry" ? "Retrying request" : "Request failed";
	}
};

/**
 * Returns a more specific title when the error is image-related.
 * Falls back to the standard kind-based title otherwise.
 */
export const getContextualErrorTitle = (
	kind: TypesGen.ChatErrorKind,
	mode: "retry" | "error",
	error?: { message: string; kind: TypesGen.ChatErrorKind; detail?: string },
): string => {
	if (error && isImageRelatedError(error)) {
		return "Image error";
	}
	return getErrorTitle(kind, mode);
};

export const getProviderStatusURL = (
	kind: TypesGen.ChatErrorKind,
	provider?: string,
): string | undefined => {
	if (kind !== "overloaded") {
		return undefined;
	}
	const normalized = normalizeProvider(provider);
	return normalized ? PROVIDER_STATUS_URLS[normalized] : undefined;
};
