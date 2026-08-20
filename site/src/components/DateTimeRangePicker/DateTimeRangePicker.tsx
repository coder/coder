/**
 * A date-and-time range picker with quick picks. The dropdown opens as
 * a plain list of relative presets; the calendar and time fields stay
 * hidden until "Custom range" is chosen. Composed from the project's
 * Calendar, Popover, Select, and Button primitives.
 *
 * Frontend-only for now: the emitted value keeps preset identity (see
 * DateTimeRange) so the API contract can be settled separately.
 */

import { CalendarIcon, CheckIcon, ChevronDownIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
import type { DateRange as DayPickerDateRange } from "react-day-picker";
import { Button, type ButtonProps } from "#/components/Button/Button";
import { Calendar } from "#/components/Calendar/Calendar";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { cn } from "#/utils/cn";
import {
	combineDateTime,
	type DateTimeRangeValue,
	DEFAULT_QUICK_PRESETS,
	formatCustomLabel,
	type Meridiem,
	parseClockTime,
	type QuickPreset,
	toClockFields,
} from "./dateTimeRange";

interface DateTimeRangePickerProps {
	value: DateTimeRangeValue;
	onChange: (value: DateTimeRangeValue) => void;
	now?: Date;
	presets?: QuickPreset[];
	size?: ButtonProps["size"];
}

const INVALID_TIME_MESSAGE = "Enter a valid time, e.g. 09:30:00";
const RANGE_ORDER_MESSAGE = "End must be after start";

interface TimeFieldsState {
	from: string;
	fromMeridiem: Meridiem;
	to: string;
	toMeridiem: Meridiem;
}

const midnightFields = (): TimeFieldsState => ({
	from: "12:00:00",
	fromMeridiem: "AM",
	to: "12:00:00",
	toMeridiem: "AM",
});

export const DateTimeRangePicker: FC<DateTimeRangePickerProps> = ({
	value,
	onChange,
	now,
	presets,
	size = "sm",
}) => {
	const currentTime = now ?? new Date();
	const quickPresets = presets ?? DEFAULT_QUICK_PRESETS;
	const [open, setOpen] = useState(false);
	const [customExpanded, setCustomExpanded] = useState(false);
	const [selection, setSelection] = useState<DayPickerDateRange | undefined>();
	const [timeFields, setTimeFields] = useState<TimeFieldsState>(midnightFields);
	const fromTimeId = useId();
	const toTimeId = useId();

	const handleOpenChange = (next: boolean) => {
		if (next) {
			// Rebuild the draft from the committed value each time the
			// dropdown opens so a previously abandoned draft never leaks in.
			if (value.preset === undefined) {
				setCustomExpanded(true);
				setSelection({ from: value.start, to: value.end });
				const from = toClockFields(value.start);
				const to = toClockFields(value.end);
				setTimeFields({
					from: from.time,
					fromMeridiem: from.meridiem,
					to: to.time,
					toMeridiem: to.meridiem,
				});
			} else {
				setCustomExpanded(false);
				setSelection(undefined);
				setTimeFields(midnightFields());
			}
		}
		setOpen(next);
	};

	const handlePreset = (preset: QuickPreset) => {
		const { start, end } = preset.range(currentTime);
		onChange({ start, end, preset: preset.id });
		setOpen(false);
	};

	const parsedFrom = parseClockTime(timeFields.from);
	const parsedTo = parseClockTime(timeFields.to);
	const fromTimeError = parsedFrom === null ? INVALID_TIME_MESSAGE : null;
	const toTimeError = parsedTo === null ? INVALID_TIME_MESSAGE : null;

	const draftStart =
		selection?.from && parsedFrom
			? combineDateTime(selection.from, parsedFrom, timeFields.fromMeridiem)
			: null;
	const draftEnd =
		selection?.from && parsedTo
			? combineDateTime(
					selection.to ?? selection.from,
					parsedTo,
					timeFields.toMeridiem,
				)
			: null;
	const rangeError =
		draftStart && draftEnd && draftEnd.getTime() <= draftStart.getTime()
			? RANGE_ORDER_MESSAGE
			: null;
	const canApply =
		draftStart !== null && draftEnd !== null && rangeError === null;

	const apply = () => {
		if (draftStart && draftEnd) {
			onChange({ start: draftStart, end: draftEnd });
			setOpen(false);
		}
	};

	const activePreset =
		value.preset === undefined
			? undefined
			: quickPresets.find((preset) => preset.id === value.preset);
	const triggerLabel =
		activePreset?.label ?? formatCustomLabel(value.start, value.end);

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button variant="outline" size={size}>
					<CalendarIcon className="size-4 text-content-secondary" />
					<span>{triggerLabel}</span>
					<ChevronDownIcon className="size-3.5 text-content-secondary" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				className="w-auto p-0 overflow-x-hidden overflow-y-auto"
				align="start"
				onOpenAutoFocus={(e) => e.preventDefault()}
			>
				<div className="flex">
					{/* Quick picks. This list is the entire dropdown until the
					    user expands the custom range panel. */}
					<div
						className={cn(
							"flex flex-col gap-0.5 p-2 text-sm",
							customExpanded && "border-r border-border-default",
						)}
					>
						{quickPresets.map((preset) => (
							<QuickPickButton
								key={preset.id}
								label={preset.label}
								selected={!customExpanded && preset.id === activePreset?.id}
								onClick={() => handlePreset(preset)}
							/>
						))}
						<QuickPickButton
							label="Custom range"
							selected={customExpanded}
							onClick={() => setCustomExpanded(true)}
						/>
					</div>

					{customExpanded && (
						<div className="flex flex-col">
							<Calendar
								mode="range"
								selected={selection}
								onSelect={setSelection}
								defaultMonth={
									value.preset === undefined ? value.start : currentTime
								}
								today={currentTime}
							/>

							{/* From/To time fields */}
							<div className="flex flex-col gap-2 border-t border-border-default px-3 py-3">
								<TimeRow
									id={fromTimeId}
									label="From"
									time={timeFields.from}
									meridiem={timeFields.fromMeridiem}
									error={fromTimeError}
									onTimeChange={(time) =>
										setTimeFields((fields) => ({ ...fields, from: time }))
									}
									onMeridiemChange={(meridiem) =>
										setTimeFields((fields) => ({
											...fields,
											fromMeridiem: meridiem,
										}))
									}
								/>
								<TimeRow
									id={toTimeId}
									label="To"
									time={timeFields.to}
									meridiem={timeFields.toMeridiem}
									error={toTimeError}
									onTimeChange={(time) =>
										setTimeFields((fields) => ({ ...fields, to: time }))
									}
									onMeridiemChange={(meridiem) =>
										setTimeFields((fields) => ({
											...fields,
											toMeridiem: meridiem,
										}))
									}
								/>
							</div>

							{/* Apply footer */}
							<div className="flex items-center justify-end gap-2 border-t border-border-default px-3 py-2">
								{rangeError !== null && (
									<span className="mr-auto text-sm text-content-destructive">
										{rangeError}
									</span>
								)}
								<Button
									variant="subtle"
									size="sm"
									onClick={() => setOpen(false)}
								>
									Cancel
								</Button>
								<Button size="sm" onClick={apply} disabled={!canApply}>
									Apply
								</Button>
							</div>
						</div>
					)}
				</div>
			</PopoverContent>
		</Popover>
	);
};

interface QuickPickButtonProps {
	label: string;
	selected: boolean;
	onClick: () => void;
}

const QuickPickButton: FC<QuickPickButtonProps> = ({
	label,
	selected,
	onClick,
}) => (
	<button
		type="button"
		aria-pressed={selected}
		onClick={onClick}
		className={cn(
			"flex cursor-pointer items-center justify-between gap-6 rounded-md border-none outline-none",
			"bg-transparent px-3 py-1.5 text-left text-sm whitespace-nowrap transition-colors",
			"text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
			"focus-visible:ring-2 focus-visible:ring-content-link",
			selected && "bg-surface-secondary text-content-primary",
		)}
	>
		{label}
		<CheckIcon aria-hidden className={cn("size-4", !selected && "invisible")} />
	</button>
);

interface TimeRowProps {
	id: string;
	label: string;
	time: string;
	meridiem: Meridiem;
	error: string | null;
	onTimeChange: (time: string) => void;
	onMeridiemChange: (meridiem: Meridiem) => void;
}

const TimeRow: FC<TimeRowProps> = ({
	id,
	label,
	time,
	meridiem,
	error,
	onTimeChange,
	onMeridiemChange,
}) => (
	<div className="flex flex-col gap-1">
		<div className="flex items-center gap-2">
			<Label
				htmlFor={id}
				className="w-10 shrink-0 text-sm text-content-secondary"
			>
				{label}
			</Label>
			<Input
				id={id}
				value={time}
				placeholder="12:00:00"
				aria-invalid={error !== null}
				className={cn(
					"h-8 w-28 tabular-nums",
					error !== null && "border-border-destructive",
				)}
				onChange={(event) => onTimeChange(event.target.value)}
			/>
			<Select
				value={meridiem}
				onValueChange={(next) => {
					if (next === "AM" || next === "PM") {
						onMeridiemChange(next);
					}
				}}
			>
				<SelectTrigger
					aria-label={`${label} AM or PM`}
					className="h-8 w-[4.5rem]"
				>
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value="AM">AM</SelectItem>
					<SelectItem value="PM">PM</SelectItem>
				</SelectContent>
			</Select>
		</div>
		{error !== null && (
			<span className="pl-12 text-sm text-content-destructive">{error}</span>
		)}
	</div>
);
