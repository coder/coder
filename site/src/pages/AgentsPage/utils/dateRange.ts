import dayjs from "dayjs";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";

/**
 * Returns true when the given date falls exactly on local midnight
 * (00:00:00.000). DateRangePicker's `toBoundary` produces local
 * midnight via `dayjs(to).startOf("day").add(1, "day").toDate()`,
 * so we use local-time methods to match that convention.
 */
function isMidnight(date: Date): boolean {
	return (
		date.getHours() === 0 &&
		date.getMinutes() === 0 &&
		date.getSeconds() === 0 &&
		date.getMilliseconds() === 0
	);
}

/**
 * When the user picks an explicit date range whose end boundary is
 * midnight of the following day, adjust it by −1 ms so the
 * DateRangePicker highlights the inclusive end date.
 */
export function toInclusiveDateRange(
	dateRange: DateRangeValue,
	endDateIsExclusive: boolean,
): DateRangeValue {
	if (endDateIsExclusive && isMidnight(dateRange.endDate)) {
		return {
			startDate: dateRange.startDate,
			endDate: new Date(dateRange.endDate.getTime() - 1),
		};
	}
	return dateRange;
}

/**
 * Format a date range for display. When `endDateIsExclusive` is true
 * and the end date is midnight, the formatted label shows the
 * preceding day.
 */
export function formatUsageDateRange(
	value: DateRangeValue,
	options?: { endDateIsExclusive?: boolean },
): string {
	const adjusted = toInclusiveDateRange(
		value,
		options?.endDateIsExclusive ?? false,
	);

	return `${dayjs(adjusted.startDate).format("MMM D")} – ${dayjs(
		adjusted.endDate,
	).format("MMM D, YYYY")}`;
}
