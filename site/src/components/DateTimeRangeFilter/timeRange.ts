import dayjs from "dayjs";
import customParseFormat from "dayjs/plugin/customParseFormat";
import { DATE_FORMAT } from "#/utils/time";

dayjs.extend(customParseFormat);

export type TimeRange = {
	startedAfter?: Date;
	startedBefore?: Date;
};

/** A range with both bounds present, as committed by the popover. */
export type FullTimeRange = {
	startedAfter: Date;
	startedBefore: Date;
};

// dayjs strict parsing is width-exact, so the format tokens are
// zero-padded only (e.g. "09:45" parses, "9:45" does not).
const DATE_FORMATS = [
	DATE_FORMAT.ISO_DATE,
	DATE_FORMAT.ISO_DATETIME,
	DATE_FORMAT.ISO_DATETIME_MINUTE,
	// T-separated datetime, as produced by the native picker and ISO paste.
	"YYYY-MM-DDTHH:mm",
];
const TIME_FORMATS = [DATE_FORMAT.TIME_24H, DATE_FORMAT.TIME_24H_MINUTE];

const NOW_PATTERN = /^now$/i;

/** Whether an expression is the literal "now" (case-insensitive). */
export const isNowExpression = (expression: string): boolean =>
	NOW_PATTERN.test(expression.trim());

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
	if (isNowExpression(trimmed)) {
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

// Bounds within a minute of the current moment read better as "now" than
// as a frozen timestamp.
const NOW_TOLERANCE_MS = 60 * 1000;

/** Whether a bound is at or near the current moment. */
export const isLiveNow = (date: Date, now: Date): boolean =>
	Math.abs(date.getTime() - now.getTime()) < NOW_TOLERANCE_MS;

const MONTH_DAY = "MMM D";

/**
 * Summarizes a resolved range the way the filter trigger displays it: a
 * single day, a range ending today, a range within one month, or a full
 * from-to range. A live "now" end bound reads as "Now" rather than "Today".
 * A partial range (one bound) reads as "Custom". Callers render "Last 24
 * hours" for the default range before falling back to this.
 */
export const formatTriggerLabel = (range: TimeRange, now: Date): string => {
	if (!range.startedAfter || !range.startedBefore) {
		return "Custom";
	}
	const from = dayjs(range.startedAfter);
	const to = range.startedBefore;
	if (dayjs(range.startedAfter).isSame(to, "day")) {
		return from.format(MONTH_DAY);
	}
	if (isLiveNow(to, now)) {
		return `${from.format(MONTH_DAY)} - Now`;
	}
	if (dayjs(to).isSame(now, "day")) {
		return `${from.format(MONTH_DAY)} - Today`;
	}
	if (
		range.startedAfter.getFullYear() === to.getFullYear() &&
		range.startedAfter.getMonth() === to.getMonth()
	) {
		return `${from.format(MONTH_DAY)} - ${to.getDate()}`;
	}
	return `${from.format(MONTH_DAY)} - ${dayjs(to).format(MONTH_DAY)}`;
};
