export type UsageSeverity = "normal" | "warning" | "exceeded";

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
