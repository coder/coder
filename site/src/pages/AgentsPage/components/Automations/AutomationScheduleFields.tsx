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

// Keeps the current hour when the shortcut fixes only the minute.
const mergeTime = (time: string, next: Partial<Time> | undefined) => {
	if (next?.minute === undefined) {
		return time;
	}
	return formatTime({
		hour: next.hour ?? parseTime(time)?.hour ?? 9,
		minute: next.minute,
	});
};

const formatRunTimeIn = (value: string, timeZone: string): string =>
	formatDate(new Date(value), {
		locale: "en-US",
		timeZone,
		timeZoneName: "short",
		weekday: "short",
		month: "short",
		day: "numeric",
		year: undefined,
		hour: "numeric",
		minute: "2-digit",
		second: undefined,
	});

// The server's tzdata can know zones that this browser's Intl rejects.
const formatRunTime = (value: string, timeZone: string): string => {
	try {
		return formatRunTimeIn(value, timeZone);
	} catch (error) {
		if (!(error instanceof RangeError)) {
			throw error;
		}
		return formatRunTimeIn(value, "UTC");
	}
};

type AutomationScheduleFieldsProps = {
	organizationId: string;
	cronField: FormHelpers;
	timeZoneField: FormHelpers;
	onCronChange: (cron: string) => void;
	onTimeZoneChange: (timeZone: string) => void;
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
}) => {
	const repeatId = useId();
	const timeId = useId();
	const cronId = useId();
	const cronDescriptionId = useId();
	const cronErrorId = useId();
	const minuteId = useId();
	const minuteErrorId = useId();
	const timeErrorId = useId();
	const [initialShortcut] = useState(() =>
		matchRepeatShortcut(String(cronField.value ?? "")),
	);
	const [repeat, setRepeat] = useState<string>(initialShortcut?.repeat ?? "");
	const [time, setTime] = useState(() => mergeTime("09:00", initialShortcut));
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
	const cronError = cronField.error ? cronField.helperText : cronPreviewError;
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
		const minute = parseCronNumber(nextMinuteText.trim(), 59);
		const parsed =
			option.value === "hourly"
				? minute === undefined
					? undefined
					: { hour: 0, minute }
				: parseTime(nextTime);
		onCronChange(parsed ? option.cron(parsed) : "");
	};

	const updateTime = (nextTime: string) => {
		setTime(nextTime);
		const minute = parseTime(nextTime)?.minute;
		if (minute !== undefined) {
			setMinuteText(String(minute));
		}
	};

	const repeatOption = repeatOptions.find((option) => option.value === repeat);
	const minuteError =
		repeat === "hourly" && parseCronNumber(minuteText.trim(), 59) === undefined
			? "Enter a minute from 0 to 59."
			: undefined;
	const timeError =
		repeatOption?.usesTime && repeat !== "hourly" && !parseTime(time)
			? "Enter a time."
			: undefined;

	let preview: React.ReactNode;
	if (!debouncedCron) {
		preview = "Enter a cron expression to see upcoming runs.";
	} else if (previewQuery.isLoading) {
		preview = (
			<span className="flex items-center gap-2">
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
			<ul className="m-0 flex list-none flex-col gap-1 p-0">
				{previewQuery.data.next_run_times.map((runTime) => (
					<li key={runTime}>{formatRunTime(runTime, debouncedTimeZone)}</li>
				))}
			</ul>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			<div className="grid grid-cols-2 gap-4">
				<div className="flex flex-col gap-2">
					<Label htmlFor={repeatId}>Repeat</Label>
					<Select
						value={repeat}
						onValueChange={(value) => {
							setRepeat(value);
							updateTime(time);
							applyShortcut(value, time);
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
					<div className="flex flex-col gap-2">
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
								const minute = parseCronNumber(nextMinuteText.trim(), 59);
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
					<div className="flex flex-col gap-2">
						<Label htmlFor={timeId}>Time</Label>
						<Input
							id={timeId}
							type="time"
							// Custom and every-N-minutes schedules have no single time.
							value={repeatOption?.usesTime ? time : ""}
							disabled={!repeatOption?.usesTime}
							aria-invalid={Boolean(timeError)}
							aria-describedby={timeError ? timeErrorId : undefined}
							onChange={(event) => {
								updateTime(event.target.value);
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
			</div>
			<div className="flex flex-col gap-2">
				<Label htmlFor={cronId}>
					Cron expression{" "}
					<span className="text-xs font-bold text-content-destructive">*</span>
				</Label>
				<div id={cronDescriptionId} className="text-xs text-content-secondary">
					Five fields: minute, hour, day of month, month, day of week.
				</div>
				<div>
					<Input
						id={cronId}
						name={cronField.name}
						value={cronField.value}
						onBlur={cronField.onBlur}
						onChange={(event) => {
							cronField.onChange(event);
							const shortcut = matchRepeatShortcut(event.target.value);
							setRepeat(shortcut?.repeat ?? "");
							updateTime(mergeTime(time, shortcut));
						}}
						required
						aria-invalid={Boolean(cronError)}
						aria-describedby={
							cronError
								? `${cronDescriptionId} ${cronErrorId}`
								: cronDescriptionId
						}
						className="font-mono"
					/>
					{/* Always rendered so screen readers announce preview errors while typing. */}
					<div aria-live="polite">
						{cronError && (
							<p
								id={cronErrorId}
								className="m-0 mt-2 text-xs text-content-destructive"
							>
								{cronError}
							</p>
						)}
					</div>
				</div>
			</div>
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
			<section aria-label="Upcoming runs" className="flex flex-col gap-2">
				<h3 className="m-0 text-sm font-medium text-content-primary">
					Upcoming runs
				</h3>
				<div className="text-sm text-content-secondary">{preview}</div>
			</section>
		</div>
	);
};

// Opens the long zone list at the selected zone instead of the top.
const scrollIntoViewOnMount = (element: HTMLElement | null) => {
	element?.scrollIntoView?.({ block: "center" });
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
	const matches = query
		? timeZones.filter((zone) => zone.toLowerCase().includes(query))
		: timeZones;
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
