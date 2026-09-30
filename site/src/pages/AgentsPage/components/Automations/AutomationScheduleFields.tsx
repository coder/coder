import { useId, useState } from "react";
import { useQuery } from "react-query";
import {
	getErrorMessage,
	getValidationErrorMessage,
	isApiValidationError,
} from "#/api/errors";
import { chatAutomationSchedulePreview } from "#/api/queries/chatAutomations";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
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
	if (!Number.isInteger(hour) || !Number.isInteger(minute)) {
		return undefined;
	}
	return { hour, minute };
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
	isCreate: boolean;
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
	isCreate,
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
	const [repeat, setRepeat] = useState<string>(isCreate ? "daily" : "");
	const [time, setTime] = useState("09:00");

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

	const applyShortcut = (nextRepeat: string, nextTime: string) => {
		const option = repeatOptions.find((o) => o.value === nextRepeat);
		const parsed = parseTime(nextTime);
		if (option && parsed) {
			onCronChange(option.cron(parsed));
		}
	};

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
				<div className="flex flex-col gap-2">
					<Label htmlFor={timeId}>Time</Label>
					<Input
						id={timeId}
						type="time"
						value={time}
						disabled={
							!repeatOptions.find((option) => option.value === repeat)?.usesTime
						}
						onChange={(event) => {
							setTime(event.target.value);
							applyShortcut(repeat, event.target.value);
						}}
					/>
				</div>
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
							setRepeat("");
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
			<SelectField
				field={{
					...timeZoneField,
					error: Boolean(timeZoneError),
					helperText: timeZoneError,
				}}
				label="Time zone"
				placeholder="Select a time zone"
				onValueChange={onTimeZoneChange}
			>
				{timeZones.map((zone) => (
					<SelectItem key={zone} value={zone}>
						{zone}
					</SelectItem>
				))}
			</SelectField>
			<section aria-label="Upcoming runs" className="flex flex-col gap-2">
				<h3 className="m-0 text-sm font-medium text-content-primary">
					Upcoming runs
				</h3>
				<div className="text-sm text-content-secondary">{preview}</div>
			</section>
		</div>
	);
};
