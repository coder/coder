import dayjs from "dayjs";
import type {
	DateTimeRangeValue,
	QuickPreset,
} from "#/components/DateTimeRangePicker/dateTimeRange";

const lastDays = (id: string, days: number): QuickPreset => ({
	id,
	label: `Last ${days} days`,
	range: (now) => ({
		start: dayjs(now).subtract(days, "day").toDate(),
		end: now,
	}),
});

const last7Days = lastDays("last_7d", 7);

/**
 * Quick picks for the spend period. Every preset fits the server's 31 day
 * limit; the picker hides the ones that reach past data retention.
 */
export const spendQuickPresets: QuickPreset[] = [
	{
		id: "last_24h",
		label: "Last 24 hours",
		range: (now) => ({
			start: dayjs(now).subtract(24, "hour").toDate(),
			end: now,
		}),
	},
	last7Days,
	lastDays("last_14d", 14),
	lastDays("last_30d", 30),
];

/** The default spend period: the 7 days ending at now. */
export const defaultSpendPeriod = (now: Date): DateTimeRangeValue => ({
	...last7Days.range(now),
	preset: last7Days.id,
});
