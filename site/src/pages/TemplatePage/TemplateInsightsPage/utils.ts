import {
	AppNameOverflow,
	type TemplateAppUsage,
	TemplateInsightsMaxUnregisteredApps,
} from "#/api/typesGenerated";
import { startOfDay, subtractTime } from "#/utils/time";

export const lastWeeks = (numberOfWeeks: number) => {
	const now = new Date();
	const endDate = subtractTime(startOfDay(now), 1, "day");
	const startDate = subtractTime(endDate, 7 * numberOfWeeks, "day");
	return { startDate, endDate };
};

/**
 * Identifies an app usage row. A session app and a template app can share a
 * slug, and the API groups template apps by slug, display name, and icon, so
 * the key takes the row type and all three.
 */
export const appUsageKey = (usage: TemplateAppUsage): string =>
	JSON.stringify([usage.type, usage.slug, usage.display_name, usage.icon]);

/**
 * Returns an app's share of the users' active time, as a percentage. Apps
 * used at the same time each count their own seconds, so shares can add up
 * past 100.
 */
export const activeTimePercentage = (
	seconds: number,
	activeSeconds: number,
): number => {
	if (activeSeconds <= 0) {
		return 0;
	}
	return Math.min(100, (seconds / activeSeconds) * 100);
};

/**
 * Explains a builtin accounting row, which totals usage without naming an
 * app. Returns undefined for every other row, including a template app whose
 * slug happens to match an accounting name.
 */
export const accountingRowDescription = (
	usage: TemplateAppUsage,
): string | undefined => {
	if (usage.type !== "builtin") {
		return undefined;
	}
	if (usage.slug === "unknown") {
		return "Usage reported without an app name, or with a reserved name. This row does not identify an individual app.";
	}
	if (usage.slug !== AppNameOverflow) {
		return undefined;
	}
	const reportLimit =
		"Time when apps past a single report's app limit were open. This row does not identify an individual app.";
	if (!usage.seconds_is_upper_bound) {
		return reportLimit;
	}
	return `${reportLimit} It also holds the unregistered apps past the ${TemplateInsightsMaxUnregisteredApps} busiest. Which minutes those apps shared is not recorded, so this is the most time they could have been open.`;
};

/** Prefixes a usage value that is only an upper bound. */
export const upperBoundLabel = (usage: TemplateAppUsage, value: string) =>
	usage.seconds_is_upper_bound ? `Up to ${value}` : value;
