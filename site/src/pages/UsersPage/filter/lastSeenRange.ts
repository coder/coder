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

/**
 * Query keys owned by the last seen picker. They share the `filter` query
 * string with the search combobox but never surface there as chips or text.
 */
export const LAST_SEEN_KEYS = [
	LAST_SEEN_AFTER_KEY,
	LAST_SEEN_BEFORE_KEY,
] as const;

export const ALL_TIME_PRESET_ID = "all_time";

const lastDays = (days: number): QuickPreset => ({
	id: `last_${days}d`,
	label: `Last ${days} days`,
	range: (now) => ({
		start: dayjs(now).subtract(days, "day").toDate(),
		end: now,
	}),
});

// "All time" applies no last seen filter. Its range only feeds the picker's
// display; the page never sends it to the API.
const allTime: QuickPreset = {
	id: ALL_TIME_PRESET_ID,
	label: "All time",
	triggerLabel: "Last seen",
	range: (now) => ({ start: new Date(0), end: now }),
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
	lastDays(30),
	lastDays(90),
];

const parseDate = (value: string | undefined): Date | undefined => {
	if (!value) {
		return undefined;
	}
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? undefined : date;
};

/**
 * The last seen range applied by the filter, or undefined when the filter has
 * no complete, valid range and every user matches.
 */
export const parseLastSeenRange = (
	values: FilterValues,
): Pick<DateTimeRangeValue, "start" | "end"> | undefined => {
	const start = parseDate(values[LAST_SEEN_AFTER_KEY]);
	const end = parseDate(values[LAST_SEEN_BEFORE_KEY]);
	if (!start || !end || start.getTime() >= end.getTime()) {
		return undefined;
	}
	return { start, end };
};

/** Filter values for a picked range; "All time" clears both keys. */
const lastSeenFilterValues = (value: DateTimeRangeValue): FilterValues =>
	value.preset === ALL_TIME_PRESET_ID
		? {}
		: {
				[LAST_SEEN_AFTER_KEY]: value.start.toISOString(),
				[LAST_SEEN_BEFORE_KEY]: value.end.toISOString(),
			};

/**
 * Replaces the last seen range in a filter query, keeping every other chip
 * and the free-text search as typed.
 */
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
