import cronParser from "cron-parser";
import cronstrue from "cronstrue";
import dayjs from "dayjs";
import { extractTimezone, stripTimezone } from "#/utils/schedule";
import { DATE_FORMAT } from "#/utils/time";

/** How often a schedule trigger runs, as picked in its editor. */
type SchedulePreset = "hourly" | "daily" | "weekdays" | "weekly" | "custom";

export const schedulePresets: readonly {
	readonly value: SchedulePreset;
	readonly label: string;
}[] = [
	{ value: "hourly", label: "Every hour" },
	{ value: "daily", label: "Every day" },
	{ value: "weekdays", label: "Every weekday" },
	{ value: "weekly", label: "Every week" },
	{ value: "custom", label: "Custom" },
];

/** Cron day of week numbers, Sunday first. */
export const scheduleWeekdays = [
	"Sunday",
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
] as const;

/** A schedule while it is edited. Fields are strings, as inputs hold them. */
export type ScheduleDraft = {
	readonly preset: SchedulePreset;
	/** "HH:mm", for daily, weekday, and weekly runs. */
	readonly time: string;
	/** Minutes past the hour, for hourly runs. */
	readonly minute: string;
	/** A cron day of week from "0" (Sunday) to "6", for weekly runs. */
	readonly weekday: string;
	/** An IANA time zone, such as Europe/Berlin. */
	readonly timeZone: string;
	/** Five cron fields without a time zone, for custom schedules. */
	readonly custom: string;
};

export type ScheduleDraftErrors = Partial<
	Record<"time" | "minute" | "custom", string>
>;

/**
 * The shortest time allowed between two runs. Keep in sync with
 * minChatProjectTriggerScheduleInterval in coderd.
 */
export const minIntervalMinutes = 5;

const pad = (value: number) => String(value).padStart(2, "0");

const normalizeFields = (fields: string) =>
	fields.trim().split(/\s+/).filter(Boolean).join(" ");

/** A whole number from `min` to `max`, or undefined. */
const parseNumber = (
	value: string | undefined,
	min: number,
	max: number,
): number | undefined => {
	if (value === undefined || !/^\d+$/.test(value)) {
		return undefined;
	}
	const number = Number(value);
	return number >= min && number <= max ? number : undefined;
};

const parseTime = (
	time: string,
): { hour: number; minute: number } | undefined => {
	const [hourText, minuteText, ...rest] = time.split(":");
	const hour = parseNumber(hourText, 0, 23);
	const minute = parseNumber(minuteText, 0, 59);
	if (rest.length > 0 || hour === undefined || minute === undefined) {
		return undefined;
	}
	return { hour, minute };
};

/** The five cron fields of a draft, without its time zone. */
const draftFields = (draft: ScheduleDraft): string => {
	if (draft.preset === "custom") {
		return normalizeFields(draft.custom);
	}
	if (draft.preset === "hourly") {
		return `${parseNumber(draft.minute, 0, 59) ?? 0} * * * *`;
	}
	const { hour, minute } = parseTime(draft.time) ?? { hour: 9, minute: 0 };
	const dayOfWeek = {
		daily: "*",
		weekdays: "1-5",
		weekly: draft.weekday,
	}[draft.preset];
	return `${minute} ${hour} * * ${dayOfWeek}`;
};

/** The cron expression to save, such as "CRON_TZ=Europe/Berlin 0 9 * * 1-5". */
export const scheduleFromDraft = (draft: ScheduleDraft): string =>
	`CRON_TZ=${draft.timeZone} ${draftFields(draft)}`;

/**
 * Reads a saved schedule back into the editor's fields. Schedules the
 * presets cannot express open as custom. Without a schedule, the draft is
 * every weekday at 9:00 AM in `defaultTimeZone`.
 */
export const draftFromSchedule = (
	schedule: string | undefined,
	defaultTimeZone: string,
): ScheduleDraft => {
	const defaults: ScheduleDraft = {
		preset: "weekdays",
		time: "09:00",
		minute: "0",
		weekday: "1",
		timeZone: defaultTimeZone,
		custom: "0 9 * * 1-5",
	};
	if (!schedule?.trim()) {
		return defaults;
	}
	const custom = normalizeFields(stripTimezone(schedule.trim()));
	const draft: ScheduleDraft = {
		...defaults,
		preset: "custom",
		// The server runs schedules without a time zone in UTC.
		timeZone: extractTimezone(schedule.trim(), "UTC"),
		custom,
	};
	const [minuteField, hourField, dayOfMonth, month, dayOfWeek, ...rest] =
		custom.split(" ");
	const minute = parseNumber(minuteField, 0, 59);
	if (
		rest.length > 0 ||
		minute === undefined ||
		dayOfMonth !== "*" ||
		month !== "*"
	) {
		return draft;
	}
	if (hourField === "*") {
		return dayOfWeek === "*"
			? { ...draft, preset: "hourly", minute: String(minute) }
			: draft;
	}
	const hour = parseNumber(hourField, 0, 23);
	if (hour === undefined) {
		return draft;
	}
	const time = `${pad(hour)}:${pad(minute)}`;
	if (dayOfWeek === "*") {
		return { ...draft, preset: "daily", time };
	}
	if (dayOfWeek === "1-5") {
		return { ...draft, preset: "weekdays", time };
	}
	const weekday = parseNumber(dayOfWeek, 0, 7);
	if (weekday === undefined) {
		return draft;
	}
	// Cron accepts 7 for Sunday too.
	return { ...draft, preset: "weekly", time, weekday: String(weekday % 7) };
};

/**
 * Switches the draft's preset. Switching to custom starts the cron
 * expression from the schedule picked so far.
 */
export const withSchedulePreset = (
	draft: ScheduleDraft,
	preset: SchedulePreset,
): ScheduleDraft => {
	if (preset === "custom" && draft.preset !== "custom") {
		return { ...draft, preset, custom: draftFields(draft) };
	}
	return { ...draft, preset };
};

/** "9:00 AM". */
const formatTime = (hour: number, minute: number) =>
	dayjs().hour(hour).minute(minute).format(DATE_FORMAT.TIME_12H);

/**
 * cronstrue pads 12-hour times, as in "09:00 AM". The presets write
 * "9:00 AM", so custom summaries drop the padding to match.
 */
const unpadHours = (description: string) =>
	description.replace(/\b0(\d:\d{2} [AP]M)\b/g, "$1");

/**
 * "Every weekday at 9:00 AM", without the time zone. Custom schedules use
 * cronstrue, such as "At 9:00 AM, on day 1 of the month".
 */
export const describeScheduleDraft = (draft: ScheduleDraft): string => {
	if (draft.preset === "hourly") {
		const minute = parseNumber(draft.minute, 0, 59) ?? 0;
		return minute === 0
			? "Every hour"
			: `Every hour at ${minute} minutes past the hour`;
	}
	if (draft.preset === "custom") {
		const fields = normalizeFields(draft.custom);
		try {
			return unpadHours(
				cronstrue.toString(fields, { throwExceptionOnParseError: true }),
			);
		} catch {
			return fields;
		}
	}
	const { hour, minute } = parseTime(draft.time) ?? { hour: 9, minute: 0 };
	const at = formatTime(hour, minute);
	if (draft.preset === "daily") {
		return `Every day at ${at}`;
	}
	if (draft.preset === "weekdays") {
		return `Every weekday at ${at}`;
	}
	const weekday = scheduleWeekdays[parseNumber(draft.weekday, 0, 6) ?? 1];
	return `Every ${weekday} at ${at}`;
};

/** "Every weekday at 9:00 AM", without the time zone. */
export const describeSchedule = (schedule: string): string =>
	describeScheduleDraft(draftFromSchedule(schedule, "UTC"));

/** Formats `date` in `timeZone` with a dayjs format such as DATE_FORMAT.FULL_DATETIME. */
export const formatInTimeZone = (
	date: Date | string,
	timeZone: string,
	format: string,
): string => dayjs(date).tz(timeZone).format(format);

/** The next runs of a valid draft after `now`, or undefined. */
const upcomingRuns = (
	draft: ScheduleDraft,
	now: Date,
	count: number,
): Date[] | undefined => {
	try {
		const expression = cronParser.parseExpression(draftFields(draft), {
			currentDate: now,
			tz: draft.timeZone,
		});
		const runs: Date[] = [];
		for (let i = 0; i < count && expression.hasNext(); i++) {
			runs.push(expression.next().toDate());
		}
		return runs;
	} catch {
		return undefined;
	}
};

/** When the draft would run next, for a preview before saving. */
export const nextScheduleRun = (
	draft: ScheduleDraft,
	now: Date,
): Date | undefined => upcomingRuns(draft, now, 1)?.[0];

/**
 * Mirrors the server's checks of the fields people type, so mistakes show
 * before saving. The server still has the final say.
 */
export const validateScheduleDraft = (
	draft: ScheduleDraft,
	now: Date,
): ScheduleDraftErrors => {
	if (draft.preset === "hourly") {
		return parseNumber(draft.minute.trim(), 0, 59) === undefined
			? { minute: "Enter a minute from 0 to 59." }
			: {};
	}
	if (draft.preset !== "custom") {
		return parseTime(draft.time) ? {} : { time: "Enter a time." };
	}
	const fields = normalizeFields(draft.custom);
	if (fields.startsWith("CRON_TZ=")) {
		return { custom: "Pick the time zone below instead of adding CRON_TZ." };
	}
	if (fields.split(" ").length !== 5) {
		return {
			custom:
				"Use five fields: minute, hour, day of month, month, and day of week.",
		};
	}
	const runs = upcomingRuns(draft, now, 50);
	if (runs === undefined) {
		return { custom: "This is not a valid cron expression." };
	}
	if (runs.length === 0) {
		return { custom: "This schedule never runs." };
	}
	const tooClose = runs.some(
		(run, i) =>
			i > 0 &&
			run.getTime() - runs[i - 1].getTime() < minIntervalMinutes * 60_000,
	);
	return tooClose
		? {
				custom: `Runs must be at least ${minIntervalMinutes} minutes apart.`,
			}
		: {};
};
