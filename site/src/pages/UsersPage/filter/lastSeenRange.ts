import dayjs from "dayjs";
import type {
	DateTimeRangeValue,
	QuickPreset,
} from "#/components/DateTimeRangePicker/dateTimeRange";
import { extractFreeText } from "#/components/Filter/FilterCombobox/filterQuery";
import {
	type FilterValues,
	parseFilterQuery,
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

export const EPOCH = new Date(0);

/** A preset with the last seen filter values it stores in the URL. */
type LastSeenPreset = QuickPreset & { filter: FilterValues };

const lastDays = (days: number): LastSeenPreset => ({
	id: `last_${days}d`,
	label: `Last ${days} days`,
	range: (now) => ({
		start: dayjs(now).subtract(days, "day").toDate(),
		end: now,
	}),
	filter: { [LAST_SEEN_AFTER_KEY]: `-${days}d` },
});

// Starts at the epoch so users who were never seen (zero last seen time) are
// excluded.
const overDays = (days: number): LastSeenPreset => ({
	id: `over_${days}d`,
	label: `Over ${days} days`,
	range: (now) => ({
		start: EPOCH,
		end: dayjs(now).subtract(days, "day").toDate(),
	}),
	filter: {
		[LAST_SEEN_AFTER_KEY]: EPOCH.toISOString(),
		[LAST_SEEN_BEFORE_KEY]: `-${days}d`,
	},
});

const allTime: LastSeenPreset = {
	id: ALL_TIME_PRESET_ID,
	label: "All time",
	placeholder: "Last seen",
	range: (now) => ({ start: EPOCH, end: now }),
	filter: {},
};

export const lastSeenPresets: LastSeenPreset[] = [
	allTime,
	{
		id: "last_24h",
		label: "Last 24 hours",
		range: (now) => ({
			start: dayjs(now).subtract(24, "hour").toDate(),
			end: now,
		}),
		filter: { [LAST_SEEN_AFTER_KEY]: "-24h" },
	},
	lastDays(7),
	overDays(30),
	overDays(90),
];

const RELATIVE_DATE_PATTERN = /^-(\d+)([hd])$/;

/** Resolves a relative value such as `-7d` or `-24h` against `now`. */
const parseRelativeDate = (
	value: string | undefined,
	now: Date,
): Date | undefined => {
	const match = value === undefined ? null : RELATIVE_DATE_PATTERN.exec(value);
	if (!match) {
		return undefined;
	}
	const unit = match[2] === "h" ? "hour" : "day";
	return dayjs(now).subtract(Number(match[1]), unit).toDate();
};

const parseLastSeenDate = (value: string | undefined, now: Date) =>
	parseRelativeDate(value, now) ?? parseDate(value);

export const parseLastSeenRange = (
	values: FilterValues,
	now: Date,
): Pick<DateTimeRangeValue, "start" | "end"> | undefined => {
	const start = parseLastSeenDate(values[LAST_SEEN_AFTER_KEY], now);
	const end = parseLastSeenDate(values[LAST_SEEN_BEFORE_KEY], now);
	if (!start || !end || start.getTime() >= end.getTime()) {
		return undefined;
	}
	return { start, end };
};

const lastSeenFilterValues = (value: DateTimeRangeValue): FilterValues => {
	const preset = lastSeenPresets.find((preset) => preset.id === value.preset);
	if (preset) {
		return preset.filter;
	}
	return {
		[LAST_SEEN_AFTER_KEY]: value.start.toISOString(),
		[LAST_SEEN_BEFORE_KEY]: value.end.toISOString(),
	};
};

const replaceLastSeen = (query: string, values: FilterValues): string =>
	[extractFreeText(query, LAST_SEEN_KEYS), stringifyFilter(values)]
		.filter((part) => part.length > 0)
		.join(" ");

/**
 * Replaces the last seen range in `query`. Presets are stored as relative
 * values so shared links stay relative; custom ranges as timestamps.
 */
export const withLastSeen = (
	query: string,
	value: DateTimeRangeValue,
): string => replaceLastSeen(query, lastSeenFilterValues(value));

/** The picker value for the last seen filter values, resolved against `now`. */
export const resolveLastSeen = (
	values: FilterValues,
	now: Date,
): DateTimeRangeValue => {
	const preset = lastSeenPresets.find(
		(preset) =>
			preset.id !== ALL_TIME_PRESET_ID &&
			LAST_SEEN_KEYS.every((key) => preset.filter[key] === values[key]),
	);
	if (preset) {
		return { ...preset.range(now), preset: preset.id };
	}
	return (
		parseLastSeenRange(values, now) ?? {
			...allTime.range(now),
			preset: ALL_TIME_PRESET_ID,
		}
	);
};

/**
 * Rewrites relative last seen values in `query` as timestamps resolved against
 * `now`, since the API only accepts timestamps.
 */
export const resolveRelativeLastSeen = (query: string, now: Date): string => {
	const values = parseFilterQuery(query);
	let changed = false;
	const resolved: FilterValues = {};
	for (const key of LAST_SEEN_KEYS) {
		const date = parseRelativeDate(values[key], now);
		changed ||= date !== undefined;
		resolved[key] = date?.toISOString() ?? values[key];
	}
	return changed ? replaceLastSeen(query, resolved) : query;
};
