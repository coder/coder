import { formatCostMicros } from "utils/currency";

/**
 * Shape of structured usage-limit fields added to 409 responses
 * from chat endpoints.
 */
interface UsageLimitData {
	spent_micros?: number;
	limit_micros?: number;
	resets_at?: string; // RFC3339
}

/**
 * Typed classification for errors surfaced in the agent detail view.
 * - "usage-limit": the user hit a spending cap (409 + valid usage data).
 * - other kinds come from normalized stream/provider failures such as
 *   "generic", "overloaded", "rate_limit", or "timeout".
 */
export type ChatDetailError = {
	message: string;
	kind:
		| "usage-limit"
		| "generic"
		| "overloaded"
		| "rate_limit"
		| "timeout"
		| (string & {});
	provider?: string;
	retryable?: boolean;
	statusCode?: number;
};

/**
 * Compare two chat-detail errors by their user-visible fields.
 */
export const chatDetailErrorsEqual = (
	left: ChatDetailError | null | undefined,
	right: ChatDetailError | null | undefined,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return false;
	}
	return (
		left.kind === right.kind &&
		left.message === right.message &&
		left.provider === right.provider &&
		left.retryable === right.retryable &&
		left.statusCode === right.statusCode
	);
};

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
 * Runtime guard that validates whether an unknown value has the shape
 * of structured usage-limit fields from a 409 response.
 * All three fields must be present with correct types.
 */
export function isUsageLimitData(value: unknown): value is UsageLimitData {
	if (value == null || typeof value !== "object") {
		return false;
	}
	const obj = value as Record<string, unknown>;
	return (
		typeof obj.spent_micros === "number" &&
		typeof obj.limit_micros === "number" &&
		typeof obj.resets_at === "string"
	);
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

	const spent = formatCostMicros(spent_micros);
	const limit = formatCostMicros(limit_micros);
	const resetDate = formatResetDate(resets_at);

	if (!resetDate) {
		return `You've used ${spent} of your ${limit} limit.`;
	}

	return `You've used ${spent} of your ${limit} limit. Resets ${resetDate}.`;
}
