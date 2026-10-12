import { cn } from "cn";
import { useId, useState } from "react";
import { useQuery } from "react-query";
import {
	getErrorMessage,
	getValidationErrorMessage,
	isApiValidationError,
} from "#/api/errors";
import { chatAutomationSchedulePreview } from "#/api/queries/chatAutomations";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { FormField } from "#/components/FormField/FormField";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDebouncedValue } from "#/hooks/debounce";
import type { FormHelpers } from "#/utils/formUtils";
import { formatDate } from "#/utils/time";
import { timeZones } from "#/utils/timeZones";

type Time = { hour: number; minute: number };

const DEFAULT_TIME: Time = { hour: 9, minute: 0 };

const repeatOptions = [
	{
		value: "every-5-minutes",
		label: "Every 5 minutes",
		usesTime: false,
		cron: () => "*/5 * * * *",
	},
	{
		value: "every-15-minutes",
		label: "Every 15 minutes",
		usesTime: false,
		cron: () => "*/15 * * * *",
	},
	{
		value: "hourly",
		label: "Hourly",
		usesTime: true,
		cron: (t: Time) => `${t.minute} * * * *`,
	},
	{
		value: "daily",
		label: "Daily",
		usesTime: true,
		cron: (t: Time) => `${t.minute} ${t.hour} * * *`,
	},
	{
		value: "weekdays",
		label: "Weekdays",
		usesTime: true,
		cron: (t: Time) => `${t.minute} ${t.hour} * * 1-5`,
	},
	{
		value: "weekly-monday",
		label: "Weekly on Monday",
		usesTime: true,
		cron: (t: Time) => `${t.minute} ${t.hour} * * 1`,
	},
] as const;

const parseTime = (value: string): Time | undefined => {
	const [hour, minute] = value.split(":").map((part) => Number(part));
	if (
		!Number.isInteger(hour) ||
		!Number.isInteger(minute) ||
		hour < 0 ||
		hour > 23 ||
		minute < 0 ||
		minute > 59
	) {
		return undefined;
	}
	return { hour, minute };
};

const formatTime = ({ hour, minute }: Time): string =>
	`${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`;

// Accepts only the digits a shortcut writes, so "09" or "1-5" do not match.
const parseCronNumber = (field: string | undefined, max: number) => {
	if (!field || !/^(0|[1-9]\d*)$/.test(field)) {
		return undefined;
	}
	const value = Number(field);
	return value <= max ? value : undefined;
};

type RepeatShortcut = {
	repeat: (typeof repeatOptions)[number]["value"];
	/** Set when the shortcut fixes the hour. */
	hour?: number;
	/** Set when the shortcut fixes the minute. */
	minute?: number;
};

/**
 * Finds the Repeat shortcut that writes exactly this cron string. It compares
 * strings and never evaluates the schedule, so equivalent forms are Custom.
 */
export const matchRepeatShortcut = (
	cron: string,
): RepeatShortcut | undefined => {
	const trimmed = cron.trim();
	const [minuteField, hourField] = trimmed.split(" ");
	const minute = parseCronNumber(minuteField, 59);
	const hour = parseCronNumber(hourField, 23);
	for (const option of repeatOptions) {
		if (!option.usesTime) {
			if (option.cron() === trimmed) {
				return { repeat: option.value };
			}
			continue;
		}
		if (minute === undefined) {
			continue;
		}
		// Hourly has no hour field, so any hour regenerates the same string.
		if (option.cron({ hour: hour ?? 0, minute }) === trimmed) {
			return option.value === "hourly"
				? { repeat: option.value, minute }
				: { repeat: option.value, hour, minute };
		}
	}
	return undefined;
};

/**
 * Describes a cron string in words when a Repeat shortcut wrote it, such as
 * "Weekdays at 09:00", or when it repeats every N minutes. Other cron strings
 * come back unchanged.
 */
export const describeSchedule = (cron: string): string => {
	const shortcut = matchRepeatShortcut(cron);
	const option = repeatOptions.find(({ value }) => value === shortcut?.repeat);
	if (!shortcut || !option) {
		const everyMinutes = /^\*\/([1-9]\d?) \* \* \* \*$/.exec(cron.trim());
		const minutes = Number(everyMinutes?.[1]);
		if (everyMinutes && minutes <= 59) {
			return minutes === 1 ? "Every minute" : `Every ${minutes} minutes`;
		}
		return cron.trim();
	}
	if (shortcut.minute === undefined) {
		return option.label;
	}
	if (shortcut.hour === undefined) {
		return `${option.label} at :${String(shortcut.minute).padStart(2, "0")}`;
	}
	return `${option.label} at ${formatTime({ hour: shortcut.hour, minute: shortcut.minute })}`;
};

const cronFieldRanges = [
	{ label: "Minute", min: 0, max: 59 },
	{ label: "Hour", min: 0, max: 23 },
	{ label: "Day of month", min: 1, max: 31 },
	{ label: "Month", min: 1, max: 12 },
	{ label: "Day of week", min: 0, max: 6 },
] as const;

/**
 * Names a problem in a cron string in words, such as "Minute must be 0-59.",
 * so the form does not show the server's parser text. It checks only the
 * field count and the numbers in each field, and returns undefined when it
 * finds nothing.
 */
export const describeCronProblem = (cron: string): string | undefined => {
	const fields = cron.trim().split(/\s+/).filter(Boolean);
	if (fields.length === 0) {
		return undefined;
	}
	if (fields.length !== cronFieldRanges.length) {
		const descriptor = fields[0].startsWith("@")
			? ` Shortcuts such as ${fields[0]} are not supported.`
			: "";
		return `Use five fields: minute, hour, day of month, month, and day of week, such as 0 9 * * 1-5.${descriptor}`;
	}
	for (const [index, field] of fields.entries()) {
		const { label, min, max } = cronFieldRanges[index];
		// A number after "/" is a step, not a value in the field's range.
		const values = field.match(/(^|[^/\d])\d+/g) ?? [];
		for (const value of values) {
			const number = Number(value.replace(/^\D/, ""));
			if (number < min || number > max) {
				return `${label} must be ${min}-${max}.`;
			}
		}
	}
	return undefined;
};

// Unlike a cron field, the Minute input accepts a leading zero ("05").
const parseMinuteInput = (text: string): number | undefined => {
	const trimmed = text.trim();
	if (!/^\d{1,2}$/.test(trimmed)) {
		return undefined;
	}
	const minute = Number(trimmed);
	return minute <= 59 ? minute : undefined;
};

// Keeps the current hour when the shortcut fixes only the minute.
const mergeTime = (time: string, next: Partial<Time> | undefined) => {
	if (next?.minute === undefined) {
		return time;
	}
	return formatTime({
		hour: next.hour ?? parseTime(time)?.hour ?? DEFAULT_TIME.hour,
		minute: next.minute,
	});
};

const formatRunTimeIn = (
	value: string,
	timeZone: string,
	timeZoneName: "short" | undefined,
): string =>
	formatDate(new Date(value), {
		locale: "en-US",
		timeZone,
		timeZoneName,
		weekday: "short",
		month: "short",
		day: "numeric",
		year: undefined,
		hour: "numeric",
		minute: "2-digit",
		second: undefined,
	});

// The line shows this many runs, as many as fit on one line.
const PREVIEW_RUN_COUNT = 3;

/**
 * Lists run times in the chosen zone, named once at the end as the user
 * picked it ("Asia/Kolkata", not "GMT+5:30"). The server's tzdata can know
 * zones that this browser's Intl rejects; those times show in UTC.
 */
const formatRunTimes = (values: readonly string[], timeZone: string) => {
	try {
		return `${values
			.map((value) => formatRunTimeIn(value, timeZone, undefined))
			.join(" · ")} (${timeZone})`;
	} catch (error) {
		if (!(error instanceof RangeError)) {
			throw error;
		}
		return values
			.map((value) => formatRunTimeIn(value, "UTC", "short"))
			.join(" · ");
	}
};

type AutomationScheduleFieldsProps = {
	organizationId: string;
	cronField: FormHelpers;
	timeZoneField: FormHelpers;
	onCronChange: (cron: string) => void;
	onTimeZoneChange: (timeZone: string) => void;
	/** Says why the previewed runs will not happen, for a saved automation that is off or paused. */
	inactiveNotice?: string;
};

/** The cron field is the stored value; upcoming runs come only from the server. */
export const AutomationScheduleFields: React.FC<
	AutomationScheduleFieldsProps
> = ({
	organizationId,
	cronField,
	timeZoneField,
	onCronChange,
	onTimeZoneChange,
	inactiveNotice,
}) => {
	const repeatId = useId();
	const timeId = useId();
	const cronId = useId();
	const cronDescriptionId = useId();
	const cronErrorId = useId();
	const intervalWarningId = useId();
	const minuteId = useId();
	const minuteErrorId = useId();
	const timeErrorId = useId();
	const [initialShortcut] = useState(() =>
		matchRepeatShortcut(String(cronField.value ?? "")),
	);
	const [repeat, setRepeat] = useState<string>(initialShortcut?.repeat ?? "");
	const [time, setTime] = useState(() =>
		mergeTime(formatTime(DEFAULT_TIME), initialShortcut),
	);
	// Separate text lets the user clear the minute while typing a new one.
	const [minuteText, setMinuteText] = useState(() =>
		String(parseTime(time)?.minute ?? 0),
	);

	const cron = String(cronField.value ?? "").trim();
	const timeZone = String(timeZoneField.value ?? "");
	const debouncedCron = useDebouncedValue(cron, 400);
	const debouncedTimeZone = useDebouncedValue(timeZone, 400);
	const previewQuery = useQuery({
		...chatAutomationSchedulePreview(organizationId, {
			schedule_cron: debouncedCron,
			schedule_time_zone: debouncedTimeZone,
		}),
		enabled: Boolean(organizationId && debouncedCron && debouncedTimeZone),
	});
	const timeZonePreviewError = isApiValidationError(previewQuery.error)
		? previewQuery.error.response.data.validations?.find(
				(validation) => validation.field === "schedule_time_zone",
			)?.detail
		: undefined;
	const cronPreviewError =
		previewQuery.isError && !timeZonePreviewError
			? getValidationErrorMessage(previewQuery.error) ||
				getErrorMessage(previewQuery.error, "Could not preview the schedule.")
			: undefined;
	// The preview describes the debounced cron. While the field holds
	// another one, its times and errors would describe the wrong schedule.
	const isPreviewCurrent =
		cron === debouncedCron && timeZone === debouncedTimeZone;
	const rawCronError = cronField.error
		? cronField.helperText
		: isPreviewCurrent
			? cronPreviewError
			: undefined;
	const cronError = rawCronError
		? (describeCronProblem(cron) ?? rawCronError)
		: undefined;
	// Only the run list renders the warning, so the cron input refers to it
	// only when that list is shown.
	const intervalWarning =
		isPreviewCurrent &&
		!previewQuery.isError &&
		previewQuery.data?.next_run_times.length
			? previewQuery.data.interval_warning
			: undefined;
	const timeZoneError = timeZoneField.error
		? timeZoneField.helperText
		: timeZonePreviewError;

	// An invalid Time or Minute clears the cron, so Save never stores a
	// schedule that differs from what the shortcut fields show.
	const applyShortcut = (
		nextRepeat: string,
		nextTime: string,
		nextMinuteText = minuteText,
	) => {
		const option = repeatOptions.find((o) => o.value === nextRepeat);
		if (!option) {
			return;
		}
		if (!option.usesTime) {
			onCronChange(option.cron());
			return;
		}
		if (option.value === "hourly") {
			const minute = parseMinuteInput(nextMinuteText);
			onCronChange(
				minute === undefined ? "" : option.cron({ hour: 0, minute }),
			);
			return;
		}
		const parsed = parseTime(nextTime);
		onCronChange(parsed ? option.cron(parsed) : "");
	};

	// The Minute input follows the minute of the last valid time.
	const minuteTextFor = (nextTime: string) => {
		const minute = parseTime(nextTime)?.minute;
		return minute === undefined ? minuteText : String(minute);
	};

	const repeatOption = repeatOptions.find((option) => option.value === repeat);
	const minuteError =
		repeat === "hourly" && parseMinuteInput(minuteText) === undefined
			? "Enter a minute from 0 to 59."
			: undefined;
	const timeError =
		repeatOption?.usesTime && repeat !== "hourly" && !parseTime(time)
			? "Enter a time."
			: undefined;

	let preview: React.ReactNode;
	if (!cron && (minuteError || timeError)) {
		// The shortcut cleared the cron because its own field is invalid.
		preview = "Upcoming runs appear when the schedule is valid.";
	} else if (!cron) {
		preview = "Enter a cron expression to see upcoming runs.";
	} else if (!isPreviewCurrent || previewQuery.isLoading) {
		preview = (
			<span className="inline-flex items-center gap-2">
				<Spinner loading size="sm" />
				Loading upcoming runs
			</span>
		);
	} else if (previewQuery.isError) {
		preview = "Upcoming runs appear when the schedule is valid.";
	} else if (!previewQuery.data?.next_run_times.length) {
		preview = "No upcoming runs.";
	} else {
		preview = (
			<>
				Next:{" "}
				{formatRunTimes(
					previewQuery.data.next_run_times.slice(0, PREVIEW_RUN_COUNT),
					debouncedTimeZone,
				)}
			</>
		);
	}
	const clockChangeNote =
		isPreviewCurrent && !previewQuery.isError
			? previewQuery.data?.clock_change_note
			: undefined;

	return (
		<div className="flex flex-col gap-3">
			{/* One row of three on wide screens, stacked on phones. */}
			<div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
				<div className="flex min-w-0 flex-col gap-2">
					<Label htmlFor={repeatId}>Repeat</Label>
					<Select
						value={repeat}
						onValueChange={(value) => {
							const nextMinuteText = minuteTextFor(time);
							setRepeat(value);
							setMinuteText(nextMinuteText);
							applyShortcut(value, time, nextMinuteText);
						}}
					>
						<SelectTrigger id={repeatId}>
							<SelectValue placeholder="Custom" />
						</SelectTrigger>
						<SelectContent>
							{repeatOptions.map((option) => (
								<SelectItem key={option.value} value={option.value}>
									{option.label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				{repeat === "hourly" ? (
					<div className="flex min-w-0 flex-col gap-2">
						<Label htmlFor={minuteId}>Minute</Label>
						<Input
							id={minuteId}
							type="number"
							min={0}
							max={59}
							step={1}
							value={minuteText}
							aria-invalid={Boolean(minuteError)}
							aria-describedby={minuteError ? minuteErrorId : undefined}
							onChange={(event) => {
								const nextMinuteText = event.target.value;
								setMinuteText(nextMinuteText);
								const minute = parseMinuteInput(nextMinuteText);
								const nextTime =
									minute === undefined ? time : mergeTime(time, { minute });
								setTime(nextTime);
								applyShortcut(repeat, nextTime, nextMinuteText);
							}}
						/>
						{minuteError && (
							<p
								id={minuteErrorId}
								className="m-0 text-xs text-content-destructive"
							>
								{minuteError}
							</p>
						)}
					</div>
				) : (
					<div className="flex min-w-0 flex-col gap-2">
						<Label htmlFor={timeId}>Time</Label>
						<Input
							id={timeId}
							type="time"
							// Custom and every-N-minutes schedules have no single
							// time, so the last time stays, disabled.
							value={time}
							disabled={!repeatOption?.usesTime}
							aria-invalid={Boolean(timeError)}
							aria-describedby={timeError ? timeErrorId : undefined}
							onChange={(event) => {
								setTime(event.target.value);
								setMinuteText(minuteTextFor(event.target.value));
								applyShortcut(repeat, event.target.value);
							}}
						/>
						{timeError && (
							<p
								id={timeErrorId}
								className="m-0 text-xs text-content-destructive"
							>
								{timeError}
							</p>
						)}
					</div>
				)}
				<FormField
					field={{
						...timeZoneField,
						error: Boolean(timeZoneError),
						helperText: timeZoneError,
					}}
					label="Time zone"
					control={(props) => (
						<TimeZoneCombobox
							{...props}
							value={timeZone}
							onChange={onTimeZoneChange}
						/>
					)}
				/>
			</div>
			{/* The cron is what is saved. It stays editable in place, so any
			    schedule the shortcuts cannot write is still one edit away. */}
			<div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
				<span className="flex min-w-0 max-w-full items-center gap-1">
					<label htmlFor={cronId} className="sr-only">
						Cron expression (required)
					</label>
					<span id={cronDescriptionId} className="sr-only">
						Five fields: minute, hour, day of month, month, day of week.
					</span>
					<input
						id={cronId}
						name={cronField.name}
						value={cronField.value}
						onBlur={cronField.onBlur}
						onChange={(event) => {
							cronField.onChange(event);
							const shortcut = matchRepeatShortcut(event.target.value);
							setRepeat(shortcut?.repeat ?? "");
							const nextTime = mergeTime(time, shortcut);
							setTime(nextTime);
							setMinuteText(minuteTextFor(nextTime));
						}}
						required
						spellCheck={false}
						placeholder="* * * * *"
						aria-invalid={Boolean(cronError)}
						aria-describedby={[
							cronDescriptionId,
							cronError && cronErrorId,
							intervalWarning && intervalWarningId,
						]
							.filter(Boolean)
							.join(" ")}
						// As wide as the expression, up to the card's width. A long
						// expression scrolls inside the input.
						style={{ width: `${Math.max(cron.length, 9) + 3}ch` }}
						className={cn(
							"h-7 min-w-0 max-w-full rounded border border-solid bg-transparent px-1.5 font-mono text-xs text-content-primary",
							"hover:border-border-secondary focus:border-border-secondary focus:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
							cronError ? "border-border-destructive" : "border-border",
						)}
					/>
					<InfoTooltip size="small" ariaLabel="About Cron expression">
						The schedule that is saved. Repeat and Time fill it in. Five fields:
						minute, hour, day of month, month, day of week.
						{clockChangeNote && (
							<span className="mt-2 block">{clockChangeNote}</span>
						)}
					</InfoTooltip>
				</span>
				<section
					aria-label="Upcoming runs"
					className="flex min-w-0 flex-col gap-1 text-content-secondary"
				>
					<span>{preview}</span>
					{inactiveNotice && (
						<span className="text-content-warning">{inactiveNotice}</span>
					)}
				</section>
			</div>
			{/* Always rendered so screen readers announce preview errors while typing. */}
			<div
				aria-live="polite"
				className="flex flex-col gap-1 text-xs empty:hidden"
			>
				{cronError && (
					<p id={cronErrorId} className="m-0 text-content-destructive">
						{cronError}
					</p>
				)}
				{intervalWarning && (
					<p id={intervalWarningId} className="m-0 text-content-warning">
						{intervalWarning}
					</p>
				)}
			</div>
		</div>
	);
};

// Opens the long zone list at the selected zone instead of the top.
const scrollIntoViewOnMount = (element: HTMLElement | null) => {
	element?.scrollIntoView({ block: "center" });
};

type TimeZoneComboboxProps = Pick<
	React.ComponentProps<"button">,
	"id" | "aria-invalid" | "aria-describedby"
> & {
	value: string;
	onChange: (timeZone: string) => void;
};

const TimeZoneCombobox: React.FC<TimeZoneComboboxProps> = ({
	value,
	onChange,
	...buttonProps
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const query = search.trim().toLowerCase();
	const matches = timeZones.filter((zone) =>
		zone.toLowerCase().includes(query),
	);
	return (
		<Combobox
			value={value}
			onValueChange={(zone) => {
				if (zone) {
					onChange(zone);
				}
			}}
			open={open}
			onOpenChange={(nextOpen) => {
				setOpen(nextOpen);
				if (!nextOpen) {
					setSearch("");
				}
			}}
		>
			<ComboboxTrigger asChild>
				<ComboboxButton
					{...buttonProps}
					selectedOption={value ? { label: value, value } : undefined}
					placeholder="Select a time zone"
				/>
			</ComboboxTrigger>
			<ComboboxContent
				shouldFilter={false}
				// Highlights the selected zone so arrow keys start from it.
				defaultValue={value}
				className="max-h-80 w-(--radix-popover-trigger-width)"
			>
				<ComboboxInput
					placeholder="Search time zones"
					value={search}
					onValueChange={setSearch}
				/>
				<ComboboxList>
					<ComboboxEmpty>No time zones found.</ComboboxEmpty>
					{matches.map((zone) => (
						<ComboboxItem key={zone} value={zone}>
							<span
								ref={zone === value ? scrollIntoViewOnMount : undefined}
								className="flex-1 truncate"
							>
								{zone}
							</span>
						</ComboboxItem>
					))}
				</ComboboxList>
			</ComboboxContent>
		</Combobox>
	);
};
