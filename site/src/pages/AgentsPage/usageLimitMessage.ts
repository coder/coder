/**
 * Shape of structured usage-limit fields added to 409 responses
 * from chat endpoints.
 */
export interface UsageLimitData {
	spent_micros?: number;
	limit_micros?: number;
	resets_at?: string; // RFC3339
}

/**
 * Convert micros (1/1,000,000 USD) to a formatted USD string.
 * Examples: 900000 → "$0.90", 50000 → "$0.05", 1500000 → "$1.50"
 */
function formatUSD(micros: number): string {
	const dollars = micros / 1_000_000;
	return dollars.toLocaleString("en-US", {
		style: "currency",
		currency: "USD",
	});
}

/**
 * Format a resets_at RFC3339 timestamp into a user-friendly string.
 * Example: "2026-03-16T00:00:00Z" → "Mar 16, 2026 at 12:00 AM"
 */
function formatResetDate(isoString: string): string {
	const date = new Date(isoString);
	if (Number.isNaN(date.getTime())) {
		return "";
	}
	return date.toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		year: "numeric",
		hour: "numeric",
		minute: "2-digit",
	});
}

/**
 * Build a user-friendly usage-limit message from structured 409
 * response data. Falls back to a generic message if structured
 * fields are missing or invalid.
 */
export function formatUsageLimitMessage(
	data: UsageLimitData,
	fallback = "Your usage limit has been reached.",
): string {
	const { spent_micros, limit_micros, resets_at } = data;

	// All structured fields must be present and valid for the
	// detailed message.
	if (
		typeof spent_micros !== "number" ||
		typeof limit_micros !== "number" ||
		typeof resets_at !== "string" ||
		!resets_at
	) {
		return fallback;
	}

	const spent = formatUSD(spent_micros);
	const limit = formatUSD(limit_micros);
	const resetDate = formatResetDate(resets_at);

	if (!resetDate) {
		return `You've used ${spent} of your ${limit} limit.`;
	}

	return `You've used ${spent} of your ${limit} limit. Resets ${resetDate}.`;
}
