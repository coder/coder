import dayjs from "dayjs";

/**
 * A quick-pick option shown at the top of the picker dropdown. Presets
 * are relative to "now" so consumers can re-resolve them on refresh
 * instead of persisting frozen timestamps.
 */
export interface QuickPreset {
	id: string;
	label: string;
	range: (now: Date) => { start: Date; end: Date };
}

/**
 * The committed picker value. Boundaries are always concrete local
 * Dates; send them to the API in UTC (Date.toISOString). Quick picks
 * resolve to dates at selection time and record their id purely so the
 * trigger can keep rendering the preset label; the id is display
 * metadata and must not be sent to the backend.
 */
export interface DateTimeRangeValue {
	start: Date;
	end: Date;
	preset?: string;
}

export const DEFAULT_QUICK_PRESETS: QuickPreset[] = [
	{
		id: "last_15m",
		label: "Last 15 min",
		range: (now) => ({
			start: dayjs(now).subtract(15, "minute").toDate(),
			end: now,
		}),
	},
	{
		id: "last_1h",
		label: "Last hour",
		range: (now) => ({
			start: dayjs(now).subtract(1, "hour").toDate(),
			end: now,
		}),
	},
	{
		id: "today",
		label: "Today",
		range: (now) => ({ start: dayjs(now).startOf("day").toDate(), end: now }),
	},
	{
		id: "this_week",
		label: "This week",
		range: (now) => ({ start: dayjs(now).startOf("week").toDate(), end: now }),
	},
];

export type Meridiem = "AM" | "PM";

interface ClockTime {
	/** 12-hour clock hours, 1-12. */
	hours: number;
	minutes: number;
	seconds: number;
}

const TIME_PATTERN = /^(\d{1,2}):([0-5]\d)(?::([0-5]\d))?$/;

/**
 * Parses a 12-hour clock string such as "12:00:00", "9:30", or
 * "09:30:15". Seconds are optional and default to zero. Returns null
 * for anything that is not a valid 12-hour time.
 */
export const parseClockTime = (text: string): ClockTime | null => {
	const match = TIME_PATTERN.exec(text.trim());
	if (!match) {
		return null;
	}
	const hours = Number(match[1]);
	if (hours < 1 || hours > 12) {
		return null;
	}
	return {
		hours,
		minutes: Number(match[2]),
		seconds: match[3] !== undefined ? Number(match[3]) : 0,
	};
};

/** Combines a calendar day with a 12-hour clock time into a local Date. */
export const combineDateTime = (
	date: Date,
	time: ClockTime,
	meridiem: Meridiem,
): Date => {
	const hours24 = (time.hours % 12) + (meridiem === "PM" ? 12 : 0);
	return new Date(
		date.getFullYear(),
		date.getMonth(),
		date.getDate(),
		hours24,
		time.minutes,
		time.seconds,
	);
};

/** Splits a Date into the picker's time text and AM/PM select values. */
export const toClockFields = (
	date: Date,
): { time: string; meridiem: Meridiem } => {
	const d = dayjs(date);
	return {
		time: d.format("hh:mm:ss"),
		meridiem: d.hour() < 12 ? "AM" : "PM",
	};
};

/**
 * Summarizes a custom range for the trigger button, favoring the most
 * compact form: "April 12", "April 10-16", "Mar 28 - Apr 2", or a
 * fully qualified pair when the years differ.
 */
export const formatCustomLabel = (start: Date, end: Date): string => {
	const from = dayjs(start);
	const to = dayjs(end);
	if (from.isSame(to, "day")) {
		return from.format("MMMM D");
	}
	if (from.isSame(to, "month")) {
		return `${from.format("MMMM D")}-${to.format("D")}`;
	}
	if (from.isSame(to, "year")) {
		return `${from.format("MMM D")} - ${to.format("MMM D")}`;
	}
	return `${from.format("MMM D, YYYY")} - ${to.format("MMM D, YYYY")}`;
};
