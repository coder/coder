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
	const end = dayjs.utc(periodEnd).format("MMMM D, YYYY");
	return `${start} - ${end}`;
}

/**
 * Classifies usage against a budget. Returns "warning" once usage reaches 85%
 * of the budget and "exceeded" once it meets or passes the budget. A budget of
 * 0 is treated as exceeded as soon as anything is used.
 */
export function getSeverity(used: number, budget: number): UsageSeverity {
	if (!Number.isFinite(used) || !Number.isFinite(budget) || budget < 0) {
		return "normal";
	}
	if (budget === 0) {
		return used > 0 ? "exceeded" : "normal";
	}
	if (used >= budget) {
		return "exceeded";
	}
	return used / budget >= 0.85 ? "warning" : "normal";
}

export function usageProgressPercentage(used: number, budget: number): number {
	if (!Number.isFinite(used) || !Number.isFinite(budget) || budget < 0) {
		return 0;
	}
	if (budget === 0) {
		return used > 0 ? 100 : 0;
	}
	return clampPercentage((used / budget) * 100);
}

export function clampPercentage(percent: number): number {
	if (!Number.isFinite(percent)) {
		return 0;
	}
	return Math.min(Math.max(percent, 0), 100);
}
