import dayjs from "dayjs";

export interface QuickPreset {
	id: string;
	label: string;
	range: (now: Date) => { start: Date; end: Date };
}

export interface DateTimeRangeValue {
	start: Date;
	end: Date;
	/** Display metadata only; never sent to the API. */
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
 * Parses a 12-hour clock string. Seconds are optional and default to zero.
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
 * Summarizes a custom range to the most compact form.
 */
export const formatCustomLabel = (start: Date, end: Date): string => {
	const from = dayjs(start);
	const to = dayjs(end);
	if (from.isSame(to, "day")) {
		return `${from.format("MMMM D, h:mm A")} - ${to.format("h:mm A")}`;
	}
	if (from.isSame(to, "month")) {
		return `${from.format("MMMM D")}-${to.format("D")}`;
	}
	if (from.isSame(to, "year")) {
		return `${from.format("MMM D")} - ${to.format("MMM D")}`;
	}
	return `${from.format("MMM D, YYYY")} - ${to.format("MMM D, YYYY")}`;
};
