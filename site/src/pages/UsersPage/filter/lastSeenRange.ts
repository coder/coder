import dayjs from "dayjs";
import type {
	DateTimeRangeValue,
	QuickPreset,
} from "#/components/DateTimeRangePicker/dateTimeRange";
import { extractFreeText } from "#/components/Filter/FilterCombobox/filterQuery";
import {
	type FilterValues,
	stringifyFilter,
} from "#/components/Filter/filterQuery";

export const LAST_SEEN_AFTER_KEY = "last_seen_after";
export const LAST_SEEN_BEFORE_KEY = "last_seen_before";

/** Query keys set by the last seen picker and hidden from the combobox. */
export const LAST_SEEN_KEYS = [
	LAST_SEEN_AFTER_KEY,
	LAST_SEEN_BEFORE_KEY,
] as const;

export const ALL_TIME_PRESET_ID = "all_time";

// Start of "Over N days" ranges, which send only `last_seen_before` so users
// who were never seen still match.
const UNBOUNDED_START = new Date(0);

const lastDays = (days: number): QuickPreset => ({
	id: `last_${days}d`,
	label: `Last ${days} days`,
	range: (now) => ({
		start: dayjs(now).subtract(days, "day").toDate(),
		end: now,
	}),
});

const overDays = (days: number): QuickPreset => ({
	id: `over_${days}d`,
	label: `Over ${days} days`,
	range: (now) => ({
		start: UNBOUNDED_START,
		end: dayjs(now).subtract(days, "day").toDate(),
	}),
});

// Sends no last seen keys; the range is only for the picker display.
const allTime: QuickPreset = {
	id: ALL_TIME_PRESET_ID,
	label: "All time",
	placeholder: "Last seen",
	range: (now) => ({ start: UNBOUNDED_START, end: now }),
};

export const lastSeenPresets: QuickPreset[] = [
	allTime,
	{
		id: "last_24h",
		label: "Last 24 hours",
		range: (now) => ({
			start: dayjs(now).subtract(24, "hour").toDate(),
			end: now,
		}),
	},
	lastDays(7),
	overDays(30),
	overDays(90),
];

const parseDate = (value: string | undefined): Date | undefined => {
	if (!value) {
		return undefined;
	}
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? undefined : date;
};

/**
 * The applied last seen range, or undefined if it is missing or invalid. A
 * range with only `last_seen_before` starts at the Unix epoch.
 */
export const parseLastSeenRange = (
	values: FilterValues,
): Pick<DateTimeRangeValue, "start" | "end"> | undefined => {
	const afterValue = values[LAST_SEEN_AFTER_KEY];
	const start = afterValue ? parseDate(afterValue) : UNBOUNDED_START;
	const end = parseDate(values[LAST_SEEN_BEFORE_KEY]);
	if (!start || !end || start.getTime() >= end.getTime()) {
		return undefined;
	}
	return { start, end };
};

const lastSeenFilterValues = (value: DateTimeRangeValue): FilterValues => {
	if (value.preset === ALL_TIME_PRESET_ID) {
		return {};
	}
	return {
		[LAST_SEEN_AFTER_KEY]:
			value.start.getTime() === UNBOUNDED_START.getTime()
				? undefined
				: value.start.toISOString(),
		[LAST_SEEN_BEFORE_KEY]: value.end.toISOString(),
	};
};

/** Replaces the last seen range in a filter query. */
export const withLastSeen = (
	query: string,
	value: DateTimeRangeValue,
): string =>
	[
		extractFreeText(query, LAST_SEEN_KEYS),
		stringifyFilter(lastSeenFilterValues(value)),
	]
		.filter((part) => part.length > 0)
		.join(" ");
