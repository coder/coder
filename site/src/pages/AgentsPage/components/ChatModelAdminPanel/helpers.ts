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
