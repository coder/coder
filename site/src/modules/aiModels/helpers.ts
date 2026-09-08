export function normalizeProvider(provider: string): string {
	return provider.trim().toLowerCase();
}

/** Display label for an effort value, e.g. "xhigh" renders as "Xhigh". */
export const formatReasoningEffort = (value: string): string =>
	value.charAt(0).toUpperCase() + value.slice(1);
