/**
 * Format cost in micros (millionths of a dollar) to a currency string.
 * Examples: 0 → "$0.00", 1_500_000 → "$1.50", 123_456 → "$0.12"
 */
export function formatCostMicros(micros: number): string {
	const dollars = micros / 1_000_000;
	if (dollars > 0 && dollars < 0.01) {
		return `$${dollars.toFixed(4)}`;
	}
	return `$${dollars.toFixed(2)}`;
}

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
		return tokens.toLocaleString();
	}
	return tokens.toString();
}
