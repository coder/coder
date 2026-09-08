import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import type {
	AgentTimeBreakdown,
	AgentTimeBucket,
	AgentTimeInterval,
} from "#/api/typesGenerated";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";

dayjs.extend(utc);

export type AgentTimeDatePreset =
	| "today"
	| "last_7_days"
	| "last_30_days"
	| "this_month"
	| "last_month"
	| "this_year"
	| "all_history"
	| "custom";

export type AgentTimeTableGroup = "organization" | "user";
export type AgentTimeSortBy = "agent_time" | "name";
export type AgentTimeSortOrder = "asc" | "desc";

const dateFormat = "YYYY-MM-DD";

export const datePresetOptions: readonly {
	value: Exclude<AgentTimeDatePreset, "custom">;
	label: string;
}[] = [
	{ value: "today", label: "Today" },
	{ value: "last_7_days", label: "Last 7 days" },
	{ value: "last_30_days", label: "Last 30 days" },
	{ value: "this_month", label: "This month" },
	{ value: "last_month", label: "Last month" },
	{ value: "this_year", label: "This year" },
	{ value: "all_history", label: "All history" },
];

export const intervalOptions: readonly {
	value: AgentTimeInterval;
	label: string;
}[] = [
	{ value: "day", label: "Daily" },
	{ value: "week", label: "Weekly" },
	{ value: "month", label: "Monthly" },
];

export function isAgentTimeTableGroup(
	value: string,
): value is AgentTimeTableGroup {
	return value === "organization" || value === "user";
}

export function isAgentTimeInterval(value: string): value is AgentTimeInterval {
	return value === "day" || value === "week" || value === "month";
}

export function isAgentTimeSortBy(value: string): value is AgentTimeSortBy {
	return value === "agent_time" || value === "name";
}

export function isAgentTimeSortOrder(
	value: string,
): value is AgentTimeSortOrder {
	return value === "asc" || value === "desc";
}

export function parseDateParam(value: string | null): string | undefined {
	if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) {
		return undefined;
	}
	const parsed = dayjs.utc(value);
	return parsed.isValid() && parsed.format(dateFormat) === value
		? value
		: undefined;
}

export function dateOnlyToLocalDate(value: string): Date {
	return dayjs(value, dateFormat).toDate();
}

export function inclusiveLocalDateFromExclusiveEnd(endDate: string): Date {
	return new Date(dateOnlyToLocalDate(endDate).getTime() - 1);
}

export function localDateToDateOnly(value: Date): string {
	return dayjs(value).format(dateFormat);
}

function isLocalMidnight(value: Date): boolean {
	return (
		value.getHours() === 0 &&
		value.getMinutes() === 0 &&
		value.getSeconds() === 0 &&
		value.getMilliseconds() === 0
	);
}

export function normalizeDateRange(value: DateRangeValue): {
	startDate: string;
	endDate: string;
} {
	const startDate = localDateToDateOnly(value.startDate);
	const endBoundary = isLocalMidnight(value.endDate)
		? dayjs(value.endDate)
		: dayjs(value.endDate).startOf("day").add(1, "day");
	return {
		startDate,
		endDate: endBoundary.format(dateFormat),
	};
}

export function tomorrowUTC(now: Date): string {
	return dayjs.utc(now).startOf("day").add(1, "day").format(dateFormat);
}

export function todayUTC(now: Date): string {
	return dayjs.utc(now).startOf("day").format(dateFormat);
}

export function presetRange(
	preset: Exclude<AgentTimeDatePreset, "custom" | "all_history">,
	now: Date,
): { startDate: string; endDate: string } {
	const today = dayjs.utc(now).startOf("day");
	const endDate = today.add(1, "day").format(dateFormat);

	switch (preset) {
		case "today":
			return { startDate: today.format(dateFormat), endDate };
		case "last_7_days":
			return {
				startDate: today.subtract(6, "day").format(dateFormat),
				endDate,
			};
		case "last_30_days":
			return {
				startDate: today.subtract(29, "day").format(dateFormat),
				endDate,
			};
		case "this_month":
			return { startDate: today.startOf("month").format(dateFormat), endDate };
		case "last_month": {
			const start = today.subtract(1, "month").startOf("month");
			return {
				startDate: start.format(dateFormat),
				endDate: start.add(1, "month").format(dateFormat),
			};
		}
		case "this_year":
			return { startDate: today.startOf("year").format(dateFormat), endDate };
	}
}

function dayCount(
	startDate: string | undefined,
	endDate: string,
): number | undefined {
	if (startDate === undefined) {
		return undefined;
	}
	const start = dayjs.utc(startDate);
	const end = dayjs.utc(endDate);
	if (!start.isValid() || !end.isValid()) {
		return undefined;
	}
	return Math.max(end.diff(start, "day"), 1);
}

export function autoInterval(
	startDate: string | undefined,
	endDate: string,
): AgentTimeInterval {
	const days = dayCount(startDate, endDate);
	if (days === undefined) {
		return "month";
	}
	if (days <= 90) {
		return "day";
	}
	if (days <= 730) {
		return "week";
	}
	return "month";
}

export function activePreset(
	startDate: string | undefined,
	endDate: string,
	now: Date,
): AgentTimeDatePreset {
	if (startDate === undefined) {
		return "all_history";
	}
	const rangePresets: readonly Exclude<
		AgentTimeDatePreset,
		"custom" | "all_history"
	>[] = [
		"today",
		"last_7_days",
		"last_30_days",
		"this_month",
		"last_month",
		"this_year",
	];

	for (const preset of rangePresets) {
		const range = presetRange(preset, now);
		if (range.startDate === startDate && range.endDate === endDate) {
			return preset;
		}
	}
	return "custom";
}

export function readSearchParam<T extends string>(
	searchParams: URLSearchParams,
	key: string,
	isAllowed: (value: string) => value is T,
	fallback: T,
): T {
	const value = searchParams.get(key);
	return value !== null && isAllowed(value) ? value : fallback;
}

export function parseAgentTimeMs(value: string | null | undefined): bigint {
	if (!value) {
		return 0n;
	}
	const trimmed = value.trim();
	if (!/^\d+$/.test(trimmed)) {
		return 0n;
	}
	return BigInt(trimmed);
}

export function formatAgentTimeHours(value: string | null | undefined): string {
	if (value === null) {
		return "Unavailable";
	}
	const ms = parseAgentTimeMs(value);
	if (ms === 0n) {
		return "0 hours";
	}
	const hundredths = (ms * 100n + 1_800_000n) / 3_600_000n;
	if (hundredths === 0n) {
		return "<0.01 hours";
	}
	const whole = hundredths / 100n;
	const fraction = (hundredths % 100n).toString().padStart(2, "0");
	if (whole >= 1_000n) {
		return `${whole.toLocaleString("en-US")} hours`;
	}
	return `${whole.toString()}.${fraction} hours`;
}

export function formatShare(value: string, total: string): string {
	const ms = parseAgentTimeMs(value);
	const totalMs = parseAgentTimeMs(total);
	if (ms === 0n || totalMs === 0n) {
		return "0%";
	}
	const basisPoints = (ms * 10_000n + totalMs / 2n) / totalMs;
	if (basisPoints === 0n) {
		return "<0.01%";
	}
	const whole = basisPoints / 100n;
	const fraction = (basisPoints % 100n).toString().padStart(2, "0");
	return fraction === "00"
		? `${whole.toString()}%`
		: `${whole.toString()}.${fraction}%`;
}

export function msToHours(value: string | null): number | null {
	if (value === null) {
		return null;
	}
	const hours = Number(parseAgentTimeMs(value)) / 3_600_000;
	return Number.isFinite(hours) ? hours : null;
}

export function formatDate(value: string): string {
	return dayjs.utc(value).format("MMM D, YYYY");
}

function formatShortDate(value: string): string {
	return dayjs.utc(value).format("MMM D");
}

export function formatBucketRange(bucket: AgentTimeBucket): string {
	const start = formatShortDate(bucket.start_date);
	const inclusiveEnd = dayjs.utc(bucket.end_date).subtract(1, "day");
	if (bucket.start_date === inclusiveEnd.format(dateFormat)) {
		return start;
	}
	return `${start} to ${inclusiveEnd.format("MMM D")}`;
}

export function formatRange(
	startDate: string | undefined,
	endDate: string,
): string {
	const end = dayjs.utc(endDate).subtract(1, "day").format("MMM D, YYYY");
	return startDate === undefined
		? `All history to ${end}`
		: `${formatDate(startDate)} to ${end}`;
}

export function formatProcessedMessages(value: string): string {
	return parseAgentTimeMs(value).toLocaleString("en-US");
}

export function approximateBucketCount(
	interval: AgentTimeInterval,
	startDate: string | undefined,
	endDate: string,
): number | undefined {
	if (startDate === undefined) {
		return undefined;
	}
	const days = dayjs.utc(endDate).diff(dayjs.utc(startDate), "day");
	if (days <= 0) {
		return 1;
	}
	if (interval === "day") {
		return days;
	}
	if (interval === "week") {
		return Math.ceil(days / 7);
	}
	return (
		dayjs.utc(endDate).diff(dayjs.utc(startDate).startOf("month"), "month") + 1
	);
}

export function intervalLabel(interval: AgentTimeInterval): string {
	return (
		intervalOptions.find((option) => option.value === interval)?.label ??
		interval
	);
}

export function entityLabel(tableGroup: AgentTimeTableGroup): string {
	return tableGroup === "organization" ? "organization" : "user";
}

export function entityHeading(tableGroup: AgentTimeTableGroup): string {
	return tableGroup === "organization" ? "Organizations" : "Users";
}

export function rowName(
	row: AgentTimeBreakdown,
	tableGroup: AgentTimeTableGroup,
): string {
	if (row.name) {
		return row.name;
	}
	return row.deleted ? `Deleted ${entityLabel(tableGroup)}` : row.id;
}

export function shortId(id: string): string {
	return id.length > 12 ? `${id.slice(0, 8)}...${id.slice(-4)}` : id;
}
