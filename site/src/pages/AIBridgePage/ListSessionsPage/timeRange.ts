import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import {
	parseFilterQuery,
	stringifyFilter,
} from "#/components/Filter/filterQuery";

dayjs.extend(utc);

/** The resolved time window a sessions query spans. */
export type TimeRange = Pick<DateTimeRangeValue, "start" | "end">;

/** Serializes a Date as RFC 3339 in UTC with second precision. */
export const toRFC3339 = (date: Date): string => {
	return dayjs(date).utc().format("YYYY-MM-DDTHH:mm:ss[Z]");
};

/** The default sessions window: the 24 hours ending at now. */
export const defaultTimeRange = (now: Date): TimeRange => ({
	start: new Date(now.getTime() - 24 * 60 * 60 * 1000),
	end: now,
});

/**
 * Appends the default time range to a filter query unless the query already
 * sets started_after or started_before. A deliberately one-sided query is
 * left alone. The default is kept in memory instead of the URL so shared
 * links resolve relative to the viewer's current time.
 */
export const withDefaultTimeRange = (
	query: string,
	range: TimeRange,
): string => {
	const values = parseFilterQuery(query);
	if (
		values.started_after !== undefined ||
		values.started_before !== undefined
	) {
		return query;
	}
	const suffix = stringifyFilter({
		...values,
		started_after: toRFC3339(range.start),
		started_before: toRFC3339(range.end),
	});
	return suffix;
};

/**
 * Builds a filter query string that replaces any existing time range in
 * values with the given range while preserving all other filters.
 * stringifyFilter quotes the RFC 3339 values because the backend query
 * parser treats unquoted colons as key/value separators.
 */
export const queryWithTimeRange = (
	values: Record<string, string | undefined>,
	range: TimeRange,
): string => {
	return stringifyFilter({
		...values,
		started_after: toRFC3339(range.start),
		started_before: toRFC3339(range.end),
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
	const start = new Date(after);
	const end = new Date(before);
	if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
		return null;
	}
	return { start, end };
};
