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
import { parseDate } from "#/utils/time";

export const LAST_SEEN_AFTER_KEY = "last_seen_after";
export const LAST_SEEN_BEFORE_KEY = "last_seen_before";

export const LAST_SEEN_KEYS = [
	LAST_SEEN_AFTER_KEY,
	LAST_SEEN_BEFORE_KEY,
] as const;

export const ALL_TIME_PRESET_ID = "all_time";

export const LAST_SEEN_PRESET_PARAM = "last_seen";

export const EPOCH = new Date(0);

const lastDays = (days: number): QuickPreset => ({
	id: `last_${days}d`,
	label: `Last ${days} days`,
	range: (now) => ({
		start: dayjs(now).subtract(days, "day").toDate(),
		end: now,
	}),
});

// Starts at the epoch so users who were never seen (zero last seen time) are
// excluded.
const overDays = (days: number): QuickPreset => ({
	id: `over_${days}d`,
	label: `Over ${days} days`,
	range: (now) => ({
		start: EPOCH,
		end: dayjs(now).subtract(days, "day").toDate(),
	}),
});

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

/** A preset ID is resolved against `now`; otherwise the filter range is used. */
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

/** Presets are stored by ID, custom ranges as timestamps in the filter. */
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
