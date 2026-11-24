import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";

dayjs.extend(utc);

export type UsageSeverity = "normal" | "warning" | "exceeded";

/**
 * Formats an AI budget window in UTC, e.g. "June 1 - July 1, 2026". Renders
 * the exclusive period_end as-is.
 */
export function formatSpendPeriodLabel(
	periodStart: string,
	periodEnd: string,
): string {
	const start = dayjs.utc(periodStart).format("MMMM D");
	return `${start} - ${formatSpendPeriodEnd(periodEnd)}`;
}

/**
 * Formats the exclusive end of an AI budget window in UTC, e.g.
 * "July 1, 2026", the day spend resets.
 */
export function formatSpendPeriodEnd(periodEnd: string): string {
	return dayjs.utc(periodEnd).format("MMMM D, YYYY");
}

/**
 * Classifies usage against a budget. Returns "warning" once usage reaches 85%
 * of the budget and "exceeded" once it meets or passes the budget. A budget of
 * 0 is exceeded even with no usage, because AI Gateway blocks requests once
 * spend reaches the limit.
 */
export function getSeverity(used: number, budget: number): UsageSeverity {
	if (!Number.isFinite(used) || !Number.isFinite(budget) || budget < 0) {
		return "normal";
	}
	if (used >= budget) {
		return "exceeded";
	}
	return used / budget >= 0.85 ? "warning" : "normal";
}

/**
 * Returns usage as a percentage of the budget, clamped from 0 to 100. A budget
 * of 0 is full, matching getSeverity.
 */
export function usageProgressPercentage(used: number, budget: number): number {
	if (!Number.isFinite(used) || !Number.isFinite(budget) || budget < 0) {
		return 0;
	}
	if (budget === 0) {
		return 100;
	}
	return clampPercentage((used / budget) * 100);
}

export function clampPercentage(percent: number): number {
	if (!Number.isFinite(percent)) {
		return 0;
	}
	return Math.min(Math.max(percent, 0), 100);
}
