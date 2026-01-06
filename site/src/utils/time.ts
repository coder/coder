import dayjs, { type Dayjs } from "dayjs";
import duration from "dayjs/plugin/duration";
import relativeTimePlugin from "dayjs/plugin/relativeTime";
import timezone from "dayjs/plugin/timezone";
import utc from "dayjs/plugin/utc";
import humanizeDuration from "humanize-duration";

// Load required plugins
dayjs.extend(duration);
dayjs.extend(relativeTimePlugin);
dayjs.extend(utc);
dayjs.extend(timezone);

// Time conversion constants
const TIME_CONSTANTS = {
	MS_PER_SECOND: 1000,
	MS_PER_MINUTE: 60 * 1000,
	MS_PER_HOUR: 60 * 60 * 1000,
	MS_PER_DAY: 24 * 60 * 60 * 1000,
	NS_PER_MS: 1e6,
	SECONDS_PER_MINUTE: 60,
	MINUTES_PER_HOUR: 60,
	HOURS_PER_DAY: 24,
};

export type TimeUnit = "days" | "hours";
type DateTimeInput = Date | string | number | Dayjs | null | undefined;

// Standard format strings
// https://day.js.org/docs/en/display/format
export const DATE_FORMAT = {
	ISO_DATE: "YYYY-MM-DD",
	ISO_DATETIME: "YYYY-MM-DD HH:mm:ss",
	FULL_DATE: "MMMM D, YYYY",
	MEDIUM_DATE: "MMM D, YYYY",
	FULL_DATETIME: "MMMM D, YYYY h:mm A",
	SHORT_DATE: "MM/DD/YYYY",
	TIME_24H: "HH:mm:ss",
	TIME_12H: "h:mm A",
	UTC_OFFSET: "Z",
} as const;

// Format functions
export function formatDateTime(
	date: DateTimeInput,
	format: string = DATE_FORMAT.ISO_DATETIME,
) {
	return dayjs(date).format(format);
}

const defaultDateLocaleOptions: Intl.DateTimeFormatOptions = {
	second: "2-digit",
	minute: "2-digit",
	hour: "2-digit",
	day: "numeric",
	month: "short",
	year: "numeric",
};

export function formatDate(
	date: Date,
	options?: { locale: Intl.LocalesArgument } & Intl.DateTimeFormatOptions,
) {
	return date.toLocaleDateString(options?.locale, {
		...defaultDateLocaleOptions,
		...options,
	});
}

// Duration functions
export function humanDuration(durationInMs: number) {
	return humanizeDuration(durationInMs, {
		conjunction: " and ",
		serialComma: false,
		round: true,
		units: ["y", "mo", "w", "d", "h", "m", "s", "ms"],
		largest: 3,
	});
}

export function durationInHours(durationMs: number): number {
	return durationMs / TIME_CONSTANTS.MS_PER_HOUR;
}

export function durationInDays(durationMs: number): number {
	return durationMs / TIME_CONSTANTS.MS_PER_DAY;
}

export function suggestedTimeUnit(duration: number): TimeUnit {
	if (duration === 0) {
		return "hours";
	}

	return Number.isInteger(durationInDays(duration)) ? "days" : "hours";
}

// Relative time functions
export function relativeTime(date: DateTimeInput) {
	return dayjs(date).fromNow();
}

export function relativeTimeWithoutSuffix(date: DateTimeInput) {
	return dayjs(date).fromNow(true);
}

export function timeFrom(
	date: DateTimeInput,
	referenceDate: DateTimeInput = new Date(),
) {
	return dayjs(date).from(dayjs(referenceDate));
}

// Time manipulation functions
export function addTime(
	date: DateTimeInput,
	amount: number,
	unit: dayjs.ManipulateType,
) {
	return dayjs(date).add(amount, unit).toDate();
}

export function subtractTime(
	date: DateTimeInput,
	amount: number,
	unit: dayjs.ManipulateType,
) {
	return dayjs(date).subtract(amount, unit).toDate();
}

export function startOfDay(date: DateTimeInput) {
	return dayjs(date).startOf("day").toDate();
}

export function startOfHour(date: DateTimeInput) {
	return dayjs(date).startOf("hour").toDate();
}

export function daysAgo(count: number) {
	return dayjs().subtract(count, "day").toISOString();
}

// Date comparison functions
export function isAfter(date1: DateTimeInput, date2: DateTimeInput) {
	return dayjs(date1).isAfter(dayjs(date2));
}
