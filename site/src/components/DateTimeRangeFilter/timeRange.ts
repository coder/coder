import dayjs from "dayjs";
import customParseFormat from "dayjs/plugin/customParseFormat";
import { DATE_FORMAT } from "#/utils/time";

dayjs.extend(customParseFormat);

export type TimeRange = {
	startedAfter: Date;
	startedBefore: Date;
};

// dayjs strict parsing is width-exact, so the format tokens are
// zero-padded only (e.g. "09:45" parses, "9:45" does not).
const DATE_FORMATS = [
	DATE_FORMAT.ISO_DATE,
	DATE_FORMAT.ISO_DATETIME,
	DATE_FORMAT.ISO_DATETIME_MINUTE,
];
const TIME_FORMATS = [DATE_FORMAT.TIME_24H, DATE_FORMAT.TIME_24H_MINUTE];

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
