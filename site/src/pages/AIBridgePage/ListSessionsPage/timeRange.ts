import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { extractFreeText } from "#/components/Filter/FilterCombobox/filterQuery";
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

const TIME_RANGE_KEYS = ["started_after", "started_before"];

const joinQuery = (...parts: string[]): string =>
	parts
		.map((part) => part.trim())
		.filter((part) => part.length > 0)
		.join(" ");

// stringifyFilter quotes the RFC 3339 values because the backend query
// parser treats unquoted colons as key/value separators.
const rangeTokens = (range: TimeRange): string =>
	stringifyFilter({
		started_after: toRFC3339(range.start),
		started_before: toRFC3339(range.end),
	});

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
	return joinQuery(query, rangeTokens(range));
};

/**
 * Removes the started_after and started_before tokens from a filter query,
 * keeping every other token and search text verbatim.
 */
export const withoutTimeRange = (query: string): string => {
	return extractFreeText(query, TIME_RANGE_KEYS);
};

/**
 * Sets both time bounds of a filter query to the given range, keeping every
 * other token and search text.
 */
export const queryWithTimeRange = (query: string, range: TimeRange): string => {
	return joinQuery(withoutTimeRange(query), rangeTokens(range));
};

/**
 * Replaces everything but the time bounds of a filter query with `search`.
 * A time bound typed in `search` overrides that bound and keeps the other.
 */
export const queryWithSearch = (query: string, search: string): string => {
	const values = parseFilterQuery(query);
	const searchValues = parseFilterQuery(search);
	return joinQuery(
		withoutTimeRange(search),
		stringifyFilter({
			started_after: searchValues.started_after ?? values.started_after,
			started_before: searchValues.started_before ?? values.started_before,
		}),
	);
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
