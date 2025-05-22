import {
	type Duration,
	add,
	differenceInDays,
	differenceInHours,
	differenceInMinutes,
	differenceInSeconds,
	format,
	formatDistanceToNow,
	parseISO,
} from "date-fns";
import { utcToZonedTime } from "date-fns-tz";

export type TimeUnit = "days" | "hours";

/**
 * Type that can be converted to a Date object
 * Used internally to standardize date handling
 */
export type DateValue = string | Date | number;

/**
 * Converts any date-like value to a Date object
 * @param value The value to convert (string, Date, or number)
 * @returns A valid Date object
 */
export function toDate(value: DateValue): Date {
	if (value instanceof Date) {
		return value;
	}
	if (typeof value === "string") {
		return safeParseISO(value);
	}
	return new Date(value);
}

// Date Format Constants
export const DATE_FORMAT = {
	FULL_DATE: "MMMM d, yyyy",
	FULL_DATE_TIME: "MMMM d, yyyy 'at' h:mm a",
	ISO_DATE: "yyyy-MM-dd",
	ISO_DATE_TIME: "yyyy-MM-dd'T'HH:mm:ss.SSSxxx",
	TIME: "h:mm a",
	SHORT_DATE: "MMM d, yyyy",
	WEEKDAY_DATE: "EEEE, MMMM d",
};

/**
 * Safely parses an ISO date string, returning a fallback value if invalid
 * @param dateString The ISO date string to parse
 * @param fallback The fallback date (defaults to current date)
 * @returns A valid Date object
 */
export function safeParseISO(dateString?: string, fallback = new Date()): Date {
	if (!dateString) {
		return fallback;
	}

	try {
		const date = parseISO(dateString);
		return !Number.isNaN(date.getTime()) ? date : fallback;
	} catch (e) {
		return fallback;
	}
}

/**
 * Formats a date according to the specified format
 * @param date Date to format
 * @param dateFormat Format to use (from DATE_FORMAT constant)
 * @returns Formatted date string
 */
export function formatDate(
	date: DateValue,
	dateFormat = DATE_FORMAT.FULL_DATE,
): string {
	return format(toDate(date), dateFormat);
}

/**
 * Returns a human-readable relative time (e.g., "2 days ago")
 * @param date Date to calculate relative time from
 * @param options Options for formatDistanceToNow
 * @returns Formatted relative time string
 */
export function relativeTime(date: DateValue, options = { addSuffix: true }) {
	return formatDistanceToNow(toDate(date), options);
}

/**
 * Formats a duration in milliseconds to a human-readable string
 * @param durationInMs Duration in milliseconds
 * @returns Formatted duration string
 */
export function humanDuration(durationInMs: number) {
	if (durationInMs === 0) {
		return "0 hours";
	}

	const timeUnit = suggestedTimeUnit(durationInMs);
	const durationValue =
		timeUnit === "days"
			? durationInDays(durationInMs)
			: durationInHours(durationInMs);

	return `${durationValue} ${timeUnit}`;
}

/**
 * Suggests an appropriate time unit based on the duration
 * @param duration Duration in milliseconds
 * @returns Suggested time unit ("days" or "hours")
 */
export function suggestedTimeUnit(duration: number): TimeUnit {
	if (duration === 0) {
		return "hours";
	}

	return Number.isInteger(durationInDays(duration)) ? "days" : "hours";
}

/**
 * Converts a duration in milliseconds to hours
 * @param duration Duration in milliseconds
 * @returns Duration in hours
 */
export function durationInHours(duration: number): number {
	return duration / 1000 / 60 / 60;
}

/**
 * Converts a duration in milliseconds to days
 * @param duration Duration in milliseconds
 * @returns Duration in days
 */
export function durationInDays(duration: number): number {
	return duration / 1000 / 60 / 60 / 24;
}

/**
 * Gets a date that was a specific number of days ago
 * @param count Number of days ago
 * @returns ISO string representation of the date
 */
export function daysAgo(count: number) {
	const date = new Date();
	date.setDate(date.getDate() - count);
	return date.toISOString();
}

/**
 * Adds a specified duration to a date
 * @param date The base date
 * @param duration The duration to add
 * @returns A new date with the duration added
 */
export function addToDate(date: DateValue, duration: Duration): Date {
	return add(toDate(date), duration);
}

/**
 * Gets the difference between two dates in the specified unit
 * @param dateLeft First date
 * @param dateRight Second date
 * @param unit Unit for difference calculation
 * @returns Difference in the specified unit
 */
export function getDateDifference(
	dateLeft: DateValue,
	dateRight: DateValue,
	unit: "seconds" | "minutes" | "hours" | "days" = "seconds",
): number {
	const dateLeftObj = toDate(dateLeft);
	const dateRightObj = toDate(dateRight);

	switch (unit) {
		case "days":
			return differenceInDays(dateLeftObj, dateRightObj);
		case "hours":
			return differenceInHours(dateLeftObj, dateRightObj);
		case "minutes":
			return differenceInMinutes(dateLeftObj, dateRightObj);
		case "seconds":
			return differenceInSeconds(dateLeftObj, dateRightObj);
		default:
			return differenceInSeconds(dateLeftObj, dateRightObj);
	}
}

/**
 * Formats a time with timezone consideration
 * @param date The date to format
 * @param timeZone The timezone to use
 * @param dateFormat The date format to apply
 * @returns A formatted date string in the specified timezone
 */
export function formatTimeWithTimezone(
	date: DateValue,
	timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone,
	dateFormat = DATE_FORMAT.FULL_DATE_TIME,
): string {
	const zonedDate = utcToZonedTime(toDate(date), timeZone);
	return format(zonedDate, dateFormat);
}
