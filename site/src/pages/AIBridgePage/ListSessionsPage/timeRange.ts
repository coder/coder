import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import {
	parseFilterQuery,
	stringifyFilter,
} from "#/components/Filter/filterQuery";

dayjs.extend(utc);

const readableTimeRangePattern =
	/(\d{4}\/\d{2}\/\d{2}\/\d{2}:\d{2})\s+to\s+(\d{4}\/\d{2}\/\d{2}\/\d{2}:\d{2})/g;
const timeRangeQueryPattern =
	/started_after:"([^"]+)"\s+started_before:"([^"]+)"/g;

const formatReadableTime = (value: string): string | null => {
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) {
		return null;
	}
	return dayjs(date).utc().format("YYYY/MM/DD/HH:mm");
};

const parseReadableTime = (value: string, seconds: number): string | null => {
	const match = /^(\d{4})\/(\d{2})\/(\d{2})\/(\d{2}):(\d{2})$/.exec(value);
	if (match === null) {
		return null;
	}
	const [, year, month, day, hour, minute] = match.map(Number);
	const date = new Date(Date.UTC(year, month - 1, day, hour, minute, seconds));
	if (
		date.getUTCFullYear() !== year ||
		date.getUTCMonth() !== month - 1 ||
		date.getUTCDate() !== day ||
		date.getUTCHours() !== hour ||
		date.getUTCMinutes() !== minute
	) {
		return null;
	}
	return dayjs(date).utc().format("YYYY-MM-DDTHH:mm:ss[Z]");
};

/** Formats verbose time bounds into a compact display-only search string. */
export const formatTimeRangeQuery = (query: string): string => {
	return query.replace(timeRangeQueryPattern, (match, startValue, endValue) => {
		const start = formatReadableTime(startValue);
		const end = formatReadableTime(endValue);
		if (start === null || end === null) {
			return match;
		}
		return `${start} to ${end}`;
	});
};

/** Parses compact display-only time bounds back into backend filter syntax. */
export const parseTimeRangeQuery = (query: string): string => {
	return query.replace(
		readableTimeRangePattern,
		(match, startValue, endValue) => {
			const start = parseReadableTime(startValue, 0);
			const end = parseReadableTime(endValue, 59);
			if (start === null || end === null) {
				return match;
			}
			return `started_after:"${start}" started_before:"${end}"`;
		},
	);
};

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
