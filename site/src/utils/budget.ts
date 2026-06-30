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

const SEVERITY_CLASSES = {
	normal: {
		text: "text-content-secondary",
		progress: "bg-content-secondary",
		ring: "stroke-content-secondary",
		border: "border-content-secondary",
	},
	warning: {
		text: "text-content-warning",
		progress: "bg-content-warning",
		ring: "stroke-content-warning",
		border: "border-content-warning",
	},
	exceeded: {
		text: "text-content-destructive",
		progress: "bg-content-destructive",
		ring: "stroke-content-destructive",
		border: "border-content-destructive",
	},
} as const satisfies Record<
	UsageSeverity,
	{ text: string; progress: string; ring: string; border: string }
>;

export function severityTextClassName(
	severity: UsageSeverity = "normal",
): string {
	return SEVERITY_CLASSES[severity].text;
}

export function severityProgressClassName(
	severity: UsageSeverity = "normal",
): string {
	return SEVERITY_CLASSES[severity].progress;
}

export function severityRingClassName(
	severity: UsageSeverity = "normal",
): string {
	return SEVERITY_CLASSES[severity].ring;
}

export function severityBorderClassName(
	severity: UsageSeverity = "normal",
): string {
	return SEVERITY_CLASSES[severity].border;
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
