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

/** Keys an app usage row. Apps reached by different methods can share a slug. */
export const appUsageKey = (usage: TemplateAppUsage): string =>
	`${usage.method}/${usage.slug}`;

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
 * Explains a session accounting row, which totals usage without naming an
 * app. Workspace apps never get one, even when their slug matches.
 */
export const accountingRowDescription = (
	usage: TemplateAppUsage,
): string | undefined => {
	if (usage.method === "workspace_app") {
		return undefined;
	}
	switch (usage.slug) {
		case "unknown":
			return "Includes all apps that did not report a name, or tried to use a reserved name.";
		case AppNameOverflow: {
			const upperBoundNote = usage.seconds_is_upper_bound
				? " Overlapping time isn't tracked, so this is the most time they could have been open."
				: "";
			return `Includes apps past the ${TemplateInsightsMaxUnregisteredApps} busiest.${upperBoundNote}`;
		}
		default:
			return undefined;
	}
};

/** Prefixes `value` with "Up to" when the row's seconds are an upper bound. */
export const upperBoundLabel = (usage: TemplateAppUsage, value: string) =>
	usage.seconds_is_upper_bound ? `Up to ${value}` : value;
