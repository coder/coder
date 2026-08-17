import dayjs from "dayjs";
import customParseFormat from "dayjs/plugin/customParseFormat";
import { stringifyFilter } from "#/components/Filter/filterQuery";

dayjs.extend(customParseFormat);

export type TimeRange = {
	startedAfter: Date;
	startedBefore: Date;
};

// dayjs strict tokens are width-exact, so both digit widths are listed.
const DATE_FORMATS = [
	"YYYY-MM-DD",
	"YYYY-MM-DD HH:mm",
	"YYYY-MM-DD HH:mm:ss",
	"YYYY-MM-DD H:mm",
	"YYYY-MM-DD H:mm:ss",
];
const TIME_FORMATS = ["HH:mm", "HH:mm:ss", "H:mm", "H:mm:ss"];

/**
 * Parses a human-friendly time expression in browser-local time:
 * "now", a clock time (current day), a date (midnight), or a date
 * with a clock time. Returns null for anything else.
 */
export const parseTimeExpression = (
	expression: string,
	now: Date,
): Date | null => {
	const trimmed = expression.trim();
	if (trimmed === "") {
		return null;
	}
	if (/^now$/i.test(trimmed)) {
		return new Date(now.getTime());
	}

	const dated = dayjs(trimmed, DATE_FORMATS, true);
	if (dated.isValid()) {
		return dated.toDate();
	}

	// Clock-only expressions resolve against the current day.
	const clock = dayjs(trimmed, TIME_FORMATS, true);
	if (clock.isValid()) {
		return new Date(
			now.getFullYear(),
			now.getMonth(),
			now.getDate(),
			clock.hour(),
			clock.minute(),
			clock.second(),
		);
	}

	return null;
};

/** Serializes a Date as RFC 3339 in UTC with second precision. */
export const toRFC3339 = (date: Date): string => {
	return date.toISOString().replace(/\.\d{3}Z$/, "Z");
};

/** The default sessions window: the 24 hours ending at now. */
export const defaultTimeRange = (now: Date): TimeRange => ({
	startedAfter: new Date(now.getTime() - 24 * 60 * 60 * 1000),
	startedBefore: now,
});

const sameDay = (a: Date, b: Date): boolean =>
	a.getFullYear() === b.getFullYear() &&
	a.getMonth() === b.getMonth() &&
	a.getDate() === b.getDate();

const MONTH_DAY = "MMM D";

/**
 * Summarizes a resolved range the way the filter trigger displays it:
 * a single day, a range ending today, a range within one month, or a
 * full from-to range. Callers render "Last 24 hours" for the default
 * range before falling back to this.
 */
export const formatTriggerLabel = (range: TimeRange, now: Date): string => {
	const from = dayjs(range.startedAfter);
	if (sameDay(range.startedAfter, range.startedBefore)) {
		return from.format(MONTH_DAY);
	}
	if (sameDay(range.startedBefore, now)) {
		return `${from.format(MONTH_DAY)} - Today`;
	}
	if (
		range.startedAfter.getFullYear() === range.startedBefore.getFullYear() &&
		range.startedAfter.getMonth() === range.startedBefore.getMonth()
	) {
		return `${from.format(MONTH_DAY)} - ${range.startedBefore.getDate()}`;
	}
	return `${from.format(MONTH_DAY)} - ${dayjs(range.startedBefore).format(MONTH_DAY)}`;
};

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
