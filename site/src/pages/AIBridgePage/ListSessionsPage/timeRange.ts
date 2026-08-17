import dayjs from "dayjs";

export type TimeRange = {
	startedAfter: Date;
	startedBefore: Date;
};

const pad = (value: number): string => String(value).padStart(2, "0");

const NOW_PATTERN = /^now$/i;
const TIME_PATTERN = /^(\d{1,2}):(\d{2})(?::(\d{2}))?$/;
const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;
const DATE_TIME_PATTERN =
	/^(\d{4})-(\d{2})-(\d{2}) (\d{1,2}):(\d{2})(?::(\d{2}))?$/;

const isValidClock = (
	hours: number,
	minutes: number,
	seconds: number,
): boolean => hours <= 23 && minutes <= 59 && seconds <= 59;

const isValidCalendarDate = (
	year: number,
	monthIndex: number,
	day: number,
): boolean => {
	const date = new Date(year, monthIndex, day);
	return (
		date.getFullYear() === year &&
		date.getMonth() === monthIndex &&
		date.getDate() === day
	);
};

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

	if (NOW_PATTERN.test(trimmed)) {
		return new Date(now.getTime());
	}

	let match = TIME_PATTERN.exec(trimmed);
	if (match) {
		const hours = Number(match[1]);
		const minutes = Number(match[2]);
		const seconds = Number(match[3] ?? 0);
		if (!isValidClock(hours, minutes, seconds)) {
			return null;
		}
		return new Date(
			now.getFullYear(),
			now.getMonth(),
			now.getDate(),
			hours,
			minutes,
			seconds,
		);
	}

	match = DATE_PATTERN.exec(trimmed);
	if (match) {
		const year = Number(match[1]);
		const monthIndex = Number(match[2]) - 1;
		const day = Number(match[3]);
		if (!isValidCalendarDate(year, monthIndex, day)) {
			return null;
		}
		return new Date(year, monthIndex, day);
	}

	match = DATE_TIME_PATTERN.exec(trimmed);
	if (match) {
		const year = Number(match[1]);
		const monthIndex = Number(match[2]) - 1;
		const day = Number(match[3]);
		const hours = Number(match[4]);
		const minutes = Number(match[5]);
		const seconds = Number(match[6] ?? 0);
		if (
			!isValidCalendarDate(year, monthIndex, day) ||
			!isValidClock(hours, minutes, seconds)
		) {
			return null;
		}
		return new Date(year, monthIndex, day, hours, minutes, seconds);
	}

	return null;
};

/** Formats a Date as a local "YYYY-MM-DD HH:mm:ss" expression. */
export const formatTimeExpression = (date: Date): string => {
	const datePart = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
	const timePart = `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
	return `${datePart} ${timePart}`;
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

const sameMonth = (a: Date, b: Date): boolean =>
	sameDay(a, b) ||
	(a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth());

const MONTH_DAY = "MMMM D";

/**
 * Summarizes a resolved range the way the filter trigger displays it:
 * a single day, a range ending today, a range within one month, or a
 * full from-to range. Callers render "Last 24 hours" for the default
 * range before falling back to this.
 */
export const formatTriggerLabel = (range: TimeRange, now: Date): string => {
	const from = dayjs(range.startedAfter);
	const to = dayjs(range.startedBefore);

	if (sameDay(range.startedAfter, range.startedBefore)) {
		return from.format(MONTH_DAY);
	}
	if (sameDay(range.startedBefore, now)) {
		return `${from.format(MONTH_DAY)} - Today`;
	}
	if (sameMonth(range.startedAfter, range.startedBefore)) {
		return `${from.format(MONTH_DAY)} - ${range.startedBefore.getDate()}`;
	}
	return `${from.format(MONTH_DAY)} - ${to.format(MONTH_DAY)}`;
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
	const suffix = [
		`started_after:"${toRFC3339(range.startedAfter)}"`,
		`started_before:"${toRFC3339(range.startedBefore)}"`,
	].join(" ");
	return query === "" ? suffix : `${query} ${suffix}`;
};

/**
 * Builds a filter query string that replaces any existing time range in
 * values with the given range while preserving all other filters. The
 * timestamps are quoted because RFC 3339 values contain colons that the
 * query parser would otherwise treat as key/value separators.
 */
export const setTimeRangeInQuery = (
	values: Record<string, string | undefined>,
	range: TimeRange,
): string => {
	const parts: string[] = [];
	for (const [key, value] of Object.entries(values)) {
		if (!value || key === "started_after" || key === "started_before") {
			continue;
		}
		parts.push(value.includes(" ") ? `${key}:"${value}"` : `${key}:${value}`);
	}
	parts.push(`started_after:"${toRFC3339(range.startedAfter)}"`);
	parts.push(`started_before:"${toRFC3339(range.startedBefore)}"`);
	return parts.join(" ");
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
