import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";

/**
 * Returns true when the given date falls exactly on midnight
 * (00:00:00.000). Date-range pickers use midnight of the *following*
 * day as the exclusive upper bound for a full-day selection. Detecting
 * this lets call sites subtract 1 ms (or 1 day) so the UI shows the
 * inclusive end date instead.
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
	hasExplicitDateRange: boolean,
): DateRangeValue {
	if (hasExplicitDateRange && isMidnight(dateRange.endDate)) {
		return {
			startDate: dateRange.startDate,
			endDate: new Date(dateRange.endDate.getTime() - 1),
		};
	}
	return dateRange;
}
