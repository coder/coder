import { cn } from "cn";
import { useId } from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { timeZones } from "#/utils/timeZones";
import { FieldHint } from "./TriggerChipEditors";
import {
	describeScheduleDraft,
	formatInTimeZone,
	minIntervalMinutes,
	nextScheduleRun,
	type ScheduleDraft,
	type ScheduleDraftErrors,
	schedulePresets,
	scheduleWeekdays,
	withSchedulePreset,
} from "./triggerSchedule";

type TriggerScheduleFieldsProps = {
	readonly draft: ScheduleDraft;
	readonly errors: ScheduleDraftErrors;
	/** Why the server did not accept the schedule, shown in place of the summary. */
	readonly serverError?: string;
	/** The time the next-run preview counts from. */
	readonly now: Date;
	/** The viewer's time zone, which the next-run preview is shown in. */
	readonly timeZone: string;
	readonly onChange: (draft: ScheduleDraft) => void;
	/** Called when Enter is pressed in a text field while the schedule has no errors. */
	readonly onSubmit?: () => void;
};

/**
 * Picks when a schedule trigger runs: a preset with its time, or a custom
 * cron expression, in a time zone. A summary shows the next run.
 */
export const TriggerScheduleFields: React.FC<TriggerScheduleFieldsProps> = ({
	draft,
	errors,
	serverError,
	now,
	timeZone,
	onChange,
	onSubmit,
}) => {
	const id = useId();
	const hasErrors = Object.keys(errors).length > 0;
	const set = (fields: Partial<ScheduleDraft>) =>
		onChange({ ...draft, ...fields });
	const submitOnEnter = (event: React.KeyboardEvent<HTMLInputElement>) => {
		if (event.key === "Enter" && !hasErrors && onSubmit) {
			event.preventDefault();
			onSubmit();
		}
	};
	// A saved zone the list lacks, such as an alias, stays selectable.
	const zones = timeZones.includes(draft.timeZone)
		? timeZones
		: [draft.timeZone, ...timeZones];
	const nextRun = hasErrors ? undefined : nextScheduleRun(draft, now);
	const isLocalZone = draft.timeZone === timeZone;

	return (
		<div className="flex flex-col gap-3">
			<div className="grid grid-cols-2 gap-2">
				<ScheduleField
					id={`${id}-preset`}
					label="Repeats"
					className={cn(draft.preset !== "weekly" && "col-span-2")}
				>
					<Select
						value={draft.preset}
						onValueChange={(value) => {
							const preset = schedulePresets.find((p) => p.value === value);
							if (preset) {
								onChange(withSchedulePreset(draft, preset.value));
							}
						}}
					>
						<SelectTrigger id={`${id}-preset`} className="h-8">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{schedulePresets.map((preset) => (
								<SelectItem key={preset.value} value={preset.value}>
									{preset.label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</ScheduleField>
				{draft.preset === "weekly" && (
					<ScheduleField id={`${id}-weekday`} label="Day">
						<Select
							value={draft.weekday}
							onValueChange={(weekday) => set({ weekday })}
						>
							<SelectTrigger id={`${id}-weekday`} className="h-8">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{scheduleWeekdays.map((name, day) => (
									<SelectItem key={name} value={String(day)}>
										{name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</ScheduleField>
				)}
				{(draft.preset === "daily" ||
					draft.preset === "weekdays" ||
					draft.preset === "weekly") && (
					<ScheduleField
						id={`${id}-time`}
						label="Time"
						error={errors.time}
						className="col-span-2"
					>
						<Input
							id={`${id}-time`}
							type="time"
							value={draft.time}
							aria-invalid={Boolean(errors.time)}
							aria-describedby={errors.time ? `${id}-time-hint` : undefined}
							className="h-8"
							onChange={(event) => set({ time: event.target.value })}
							onKeyDown={submitOnEnter}
						/>
					</ScheduleField>
				)}
				{draft.preset === "hourly" && (
					<ScheduleField
						id={`${id}-minute`}
						label="Minutes past the hour"
						error={errors.minute}
						className="col-span-2"
					>
						<Input
							id={`${id}-minute`}
							type="number"
							min={0}
							max={59}
							value={draft.minute}
							aria-invalid={Boolean(errors.minute)}
							aria-describedby={errors.minute ? `${id}-minute-hint` : undefined}
							className="h-8"
							onChange={(event) => set({ minute: event.target.value })}
							onKeyDown={submitOnEnter}
						/>
					</ScheduleField>
				)}
				{draft.preset === "custom" && (
					<ScheduleField
						id={`${id}-custom`}
						label="Cron expression"
						error={errors.custom}
						hint={`Minute, hour, day of month, month, and day of week. Runs must be at least ${minIntervalMinutes} minutes apart.`}
						className="col-span-2"
					>
						<Input
							id={`${id}-custom`}
							value={draft.custom}
							placeholder="0 9 * * 1-5"
							spellCheck={false}
							autoComplete="off"
							aria-invalid={Boolean(errors.custom)}
							aria-describedby={`${id}-custom-hint`}
							className="h-8 font-mono"
							onChange={(event) => set({ custom: event.target.value })}
							onKeyDown={submitOnEnter}
						/>
					</ScheduleField>
				)}
				<ScheduleField
					id={`${id}-zone`}
					label="Time zone"
					className="col-span-2"
				>
					<Select
						value={draft.timeZone}
						onValueChange={(zone) => set({ timeZone: zone })}
					>
						<SelectTrigger id={`${id}-zone`} className="h-8">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{zones.map((zone) => (
								<SelectItem key={zone} value={zone}>
									{zone}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</ScheduleField>
			</div>
			{serverError ? (
				<p role="alert" className="m-0 text-xs text-content-destructive">
					{serverError}
				</p>
			) : (
				!hasErrors && (
					<p className="m-0 text-xs text-content-secondary">
						{describeScheduleDraft(draft)}
						{!isLocalZone && ` (${draft.timeZone})`}.
						{nextRun && (
							<>
								{" "}
								Next run:{" "}
								<time dateTime={nextRun.toISOString()}>
									{formatInTimeZone(nextRun, timeZone, "ddd, MMM D, h:mm A")}
								</time>
								{!isLocalZone && " your time"}.
							</>
						)}
					</p>
				)
			)}
		</div>
	);
};

type ScheduleFieldProps = {
	readonly id: string;
	readonly label: string;
	readonly error?: string;
	/** Shown under the field until it has an error. */
	readonly hint?: string;
	readonly className?: string;
	readonly children: React.ReactNode;
};

/** A label over a field, and the field's hint or error. */
const ScheduleField: React.FC<ScheduleFieldProps> = ({
	id,
	label,
	error,
	hint,
	className,
	children,
}) => (
	<div className={cn("flex min-w-0 flex-col gap-1.5", className)}>
		<Label htmlFor={id}>{label}</Label>
		{children}
		{(error || hint) && (
			<FieldHint id={`${id}-hint`} error={error}>
				{hint}
			</FieldHint>
		)}
	</div>
);
