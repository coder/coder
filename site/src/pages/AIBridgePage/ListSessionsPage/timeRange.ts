import type { TimeRange } from "#/components/DateTimeRangeFilter/timeRange";
import { stringifyFilter } from "#/components/Filter/filterQuery";

export type { TimeRange } from "#/components/DateTimeRangeFilter/timeRange";

/** Serializes a Date as RFC 3339 in UTC with second precision. */
export const toRFC3339 = (date: Date): string => {
	return date.toISOString().replace(/\.\d{3}Z$/, "Z");
};

/** The default sessions window: the 24 hours ending at now. */
export const defaultTimeRange = (now: Date): TimeRange => ({
	startedAfter: new Date(now.getTime() - 24 * 60 * 60 * 1000),
	startedBefore: now,
});

const TIME_RANGE_KEY_PATTERN = /started_(after|before):/;

/**
 * Appends the default time range to a filter query unless the query already
 * sets an explicit started_after or started_before. The default is kept in
 * memory instead of the URL so shared links resolve relative to the
 * viewer's current time.
 */
export const withDefaultTimeRange = (
	query: string,
	range: TimeRange,
): string => {
	if (TIME_RANGE_KEY_PATTERN.test(query)) {
		return query;
	}
	const suffix = stringifyFilter({
		started_after: toRFC3339(range.startedAfter),
		started_before: toRFC3339(range.startedBefore),
	});
	return query === "" ? suffix : `${query} ${suffix}`;
};

/**
 * Builds a filter query string that replaces any existing time range in
 * values with the given range while preserving all other filters.
 * stringifyFilter quotes the RFC 3339 values because the backend query
 * parser treats unquoted colons as key/value separators.
 */
export const setTimeRangeInQuery = (
	values: Record<string, string | undefined>,
	range: TimeRange,
): string => {
	return stringifyFilter({
		...values,
		started_after: toRFC3339(range.startedAfter),
		started_before: toRFC3339(range.startedBefore),
	});
};

/** Extracts an explicit time range from filter values, or null if absent. */
export const parseTimeRange = (
	values: Record<string, string | undefined>,
): TimeRange | null => {
	const after = values.started_after;
	const before = values.started_before;
	if (!after || !before) {
		return null;
	}
	const startedAfter = new Date(after);
	const startedBefore = new Date(before);
	if (
		Number.isNaN(startedAfter.getTime()) ||
		Number.isNaN(startedBefore.getTime())
	) {
		return null;
	}
	return { startedAfter, startedBefore };
};
