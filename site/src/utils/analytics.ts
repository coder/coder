/**
 * Format cost in micros (millionths of a dollar) to a currency string.
 * Examples: 0 → "$0.00", 1_500_000 → "$1.50", 123_456 → "$0.12"
 */
export function formatCostMicros(micros: number | string): string {
	const microsValue = typeof micros === "string" ? Number(micros) : micros;
	if (Number.isNaN(microsValue) || !Number.isFinite(microsValue)) {
		return "$0.00";
	}

	const sign = microsValue < 0 ? "-" : "";
	const dollars = Math.abs(microsValue) / 1_000_000;
	const rounded = Number(dollars.toFixed(4));
	if (rounded > 0 && rounded < 0.01) {
		return `${sign}$${dollars.toFixed(4)}`;
	}
	return `${sign}$${dollars.toFixed(2)}`;
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
		return tokens.toLocaleString("en-US");
	}
	return tokens.toString();
}
