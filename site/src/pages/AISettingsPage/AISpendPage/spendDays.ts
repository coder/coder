import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import {
	type AISpendPeriodWindow,
	MaxAISpendPeriodDays,
} from "#/api/typesGenerated";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { DATE_FORMAT, durationInDays } from "#/utils/time";

dayjs.extend(utc);

const DEFAULT_SPEND_DAY_COUNT = 7;

/** An inclusive range of UTC days, each formatted as `YYYY-MM-DD`. */
export type SpendDays = {
	startDate: string;
	endDate: string;
};

/**
 * The default range: the last 7 UTC days, including the current one, starting
 * no earlier than `minDay`.
 */
export const defaultSpendDays = (now: Date, minDay?: string): SpendDays => {
	const today = dayjs.utc(now);
	const startDate = today
		.subtract(DEFAULT_SPEND_DAY_COUNT - 1, "day")
		.format(DATE_FORMAT.ISO_DATE);
	return {
		startDate: minDay !== undefined && minDay > startDate ? minDay : startDate,
		endDate: today.format(DATE_FORMAT.ISO_DATE),
	};
};

/**
 * The first UTC day whose data is fully retained, or undefined when data is
 * never purged.
 */
export const firstRetainedDay = (
	now: Date,
	retentionMs: number,
): string | undefined => {
	if (retentionMs <= 0) {
		return undefined;
	}
	const cutoff = dayjs.utc(now).subtract(retentionMs, "millisecond");
	const day = cutoff.startOf("day");
	return (day.isSame(cutoff) ? day : day.add(1, "day")).format(
		DATE_FORMAT.ISO_DATE,
	);
};

/**
 * Whether the retention is shorter than a day, so no UTC day is ever kept in
 * full.
 */
export const isUnderADay = (retentionMs: number): boolean =>
	retentionMs > 0 && durationInDays(retentionMs) < 1;

/**
 * DateRangePicker only knows local time, so this gives it a local "now" that
 * shows the UTC date and time. Its today and presets then follow UTC.
 */
export const toPickerNow = (now: Date): Date =>
	dayjs(dayjs.utc(now).format("YYYY-MM-DDTHH:mm:ss")).toDate();

const isDay = (value: string): boolean =>
	dayjs.utc(value).format(DATE_FORMAT.ISO_DATE) === value;

/**
 * Reads a range from URL values, or returns null when they are missing,
 * malformed, reversed, longer than the server allows, or start before
 * `minDay`.
 */
export const parseSpendDays = (
	startDate: string | null,
	endDate: string | null,
	minDay?: string,
): SpendDays | null => {
	if (!startDate || !endDate || !isDay(startDate) || !isDay(endDate)) {
		return null;
	}
	if (minDay !== undefined && startDate < minDay) {
		return null;
	}
	const dayCount = dayjs.utc(endDate).diff(dayjs.utc(startDate), "day") + 1;
	if (dayCount < 1 || dayCount > MaxAISpendPeriodDays) {
		return null;
	}
	return { startDate, endDate };
};

/**
 * Spend is aggregated per UTC day, so each picked calendar day maps to that
 * UTC day. The API takes an exclusive end.
 */
export const toSpendPeriod = (days: SpendDays): AISpendPeriodWindow => ({
	period_start: dayjs.utc(days.startDate).toISOString(),
	period_end: dayjs.utc(days.endDate).add(1, "day").toISOString(),
});

/** Formats a UTC day, e.g. "Mar 6". */
export const formatSpendDay = (day: string): string =>
	dayjs.utc(day).format("MMM D");

/** Formats a UTC day with its year and zone, e.g. "Mar 6, 2026 (UTC)". */
export const formatSpendDayLong = (day: string): string =>
	dayjs.utc(day).format("MMM D, YYYY [(UTC)]");

/**
 * Labels the days compactly, e.g. "Mar 6-12" or "Feb 27 - Mar 12".
 */
export const formatSpendDaysLabel = ({
	startDate,
	endDate,
}: SpendDays): string => {
	if (startDate === endDate) {
		return formatSpendDay(startDate);
	}
	const end = dayjs.utc(endDate);
	if (dayjs.utc(startDate).isSame(end, "month")) {
		return `${formatSpendDay(startDate)}-${end.format("D")}`;
	}
	return `${formatSpendDay(startDate)} - ${formatSpendDay(endDate)}`;
};

/** Converts the days into the picker's local, end-exclusive boundaries. */
export const toPickerValue = (days: SpendDays): DateRangeValue => ({
	startDate: dayjs(days.startDate).toDate(),
	endDate: dayjs(days.endDate).add(1, "day").toDate(),
});

/**
 * Reads the picked calendar days back from the picker. The picker ends a range
 * at the next local midnight, or at the next hour when the range ends today.
 */
export const fromPickerValue = (value: DateRangeValue): SpendDays => {
	const end = dayjs(value.endDate);
	const lastDay = end.isSame(end.startOf("day")) ? end.subtract(1, "day") : end;
	return {
		startDate: dayjs(value.startDate).format(DATE_FORMAT.ISO_DATE),
		endDate: lastDay.format(DATE_FORMAT.ISO_DATE),
	};
};
