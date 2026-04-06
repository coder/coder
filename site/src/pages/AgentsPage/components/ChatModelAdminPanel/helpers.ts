/**
 * Reads a value as a non-empty string, returning undefined for
 * empty strings, null, or undefined values.
 */
export function readOptionalString(value: unknown): string | undefined {
	if (typeof value !== "string") return undefined;
	const trimmed = value.trim();
	return trimmed || undefined;
}

/**
 * Normalizes a provider name for case-insensitive comparison.
 */
export function normalizeProvider(provider: string): string {
	return provider.trim().toLowerCase();
}

export const NIL_UUID = "00000000-0000-0000-0000-000000000000";

export const isDatabaseProviderConfig = (config: {
	id: string;
	source?: string;
}): boolean => {
	if (config.id === NIL_UUID) return false;
	// Env and env_preset sources are not database configs.
	// Undefined or "database" both indicate a DB-managed config.
	if (config.source !== undefined && config.source !== "database") return false;
	return true;
};
