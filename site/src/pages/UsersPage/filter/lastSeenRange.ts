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

/** Search param holding the picked preset, so its range stays relative. */
export const LAST_SEEN_PRESET_PARAM = "last_seen";

// Start of "Over N days" ranges. Users who were never seen have a zero last
// seen time, which is earlier, so they are excluded.
const EPOCH = new Date(0);

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
		start: EPOCH,
		end: dayjs(now).subtract(days, "day").toDate(),
	}),
});

// Sends no last seen keys; the range is only for the picker display.
const allTime: QuickPreset = {
	id: ALL_TIME_PRESET_ID,
	label: "All time",
	placeholder: "Last seen",
	range: (now) => ({ start: EPOCH, end: now }),
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

/** The applied last seen range, or undefined if it is missing or invalid. */
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

const lastSeenFilterValues = (value: DateTimeRangeValue): FilterValues =>
	value.preset === ALL_TIME_PRESET_ID
		? {}
		: {
				[LAST_SEEN_AFTER_KEY]: value.start.toISOString(),
				[LAST_SEEN_BEFORE_KEY]: value.end.toISOString(),
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

/**
 * The last seen range for the page. A preset from `LAST_SEEN_PRESET_PARAM` is
 * resolved against `now`; otherwise the range comes from the filter query.
 */
export const resolveLastSeen = (
	presetId: string | null,
	values: FilterValues,
	now: Date,
): DateTimeRangeValue => {
	const preset = lastSeenPresets.find(
		(preset) => preset.id === presetId && preset.id !== ALL_TIME_PRESET_ID,
	);
	if (preset) {
		return { ...preset.range(now), preset: preset.id };
	}
	return (
		parseLastSeenRange(values) ?? {
			...allTime.range(now),
			preset: ALL_TIME_PRESET_ID,
		}
	);
};

/**
 * The URL state for a picked range. Presets are stored by ID and custom
 * ranges as timestamps in the filter query.
 */
export const lastSeenUrlState = (
	query: string,
	value: DateTimeRangeValue,
): { filter: string; preset: string | undefined } => {
	if (value.preset === undefined) {
		return { filter: withLastSeen(query, value), preset: undefined };
	}
	return {
		filter: extractFreeText(query, LAST_SEEN_KEYS),
		preset: value.preset === ALL_TIME_PRESET_ID ? undefined : value.preset,
	};
};
