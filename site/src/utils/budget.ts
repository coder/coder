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

export function severityTextClassName(
	severity: UsageSeverity = "normal",
): string {
	switch (severity) {
		case "exceeded":
			return "text-content-destructive";
		case "warning":
			return "text-content-warning";
		case "normal":
			return "text-content-secondary";
	}
}
