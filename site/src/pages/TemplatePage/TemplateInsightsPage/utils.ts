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
 * Keys an app usage row. Session and template apps can share a slug, and
 * template apps are grouped by slug, display name, and icon.
 */
export const appUsageKey = (usage: TemplateAppUsage): string =>
	JSON.stringify([usage.type, usage.slug, usage.display_name, usage.icon]);

/** Returns an app's percentage of the users' active time, capped at 100. */
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
 * app. Template apps never get one, even when their slug matches.
 */
export const accountingRowDescription = (
	usage: TemplateAppUsage,
): string | undefined => {
	if (usage.type !== "builtin") {
		return undefined;
	}
	switch (usage.slug) {
		case "unknown":
			return "Usage reported without an app name, or with a reserved name. This row does not identify an individual app.";
		case AppNameOverflow: {
			const folded = usage.seconds_is_upper_bound
				? ` It also holds the unregistered apps past the ${TemplateInsightsMaxUnregisteredApps} busiest. Which minutes those apps shared is not recorded, so this is the most time they could have been open.`
				: "";
			return `Time when apps past a single report's app limit were open. This row does not identify an individual app.${folded}`;
		}
		default:
			return undefined;
	}
};

/** Prefixes `value` with "Up to" when the row's seconds are an upper bound. */
export const upperBoundLabel = (usage: TemplateAppUsage, value: string) =>
	usage.seconds_is_upper_bound ? `Up to ${value}` : value;
