/**
 * Format a token count to a compact human-readable string.
 * Examples: 0 → "0", 1234 → "1,234", 1_500_000 → "1.5M"
 */
export function formatTokenCount(tokens: number): string {
	if (tokens >= 1_000_000) {
		const millions = tokens / 1_000_000;
		return `${millions % 1 === 0 ? millions.toFixed(0) : millions.toFixed(1)}M`;
	}
	if (tokens >= 1_000) {
		return tokens.toLocaleString("en-US");
	}
	return tokens.toString();
}
