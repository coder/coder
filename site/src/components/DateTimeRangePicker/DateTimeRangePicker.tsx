/**
 * A date-and-time range picker with quick picks. The dropdown opens as
 * a plain list of relative presets; the calendar and time fields stay
 * hidden until "Custom range" is chosen. Composed from the project's
 * Calendar, Popover, Select, and Button primitives.
 *
 * Frontend-only: the emitted value is always resolved dates (see
 * DateTimeRangeValue); consumers convert to UTC at the API boundary.
 */

import { CalendarIcon, CheckIcon, ChevronDownIcon } from "lucide-react";
import { type FC, type KeyboardEvent, useId, useState } from "react";
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
	fromTouched: boolean;
	to: string;
	toMeridiem: Meridiem;
	toTouched: boolean;
}

const midnightFields = (): TimeFieldsState => ({
	from: "12:00:00",
	fromMeridiem: "AM",
	fromTouched: false,
	to: "12:00:00",
	toMeridiem: "AM",
	toTouched: false,
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
					fromTouched: false,
					to: to.time,
					toMeridiem: to.meridiem,
					toTouched: false,
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

	// Roving focus for the quick-pick radiogroup: arrows move between
	// options, Tab leaves the group from the selected item.
	const handleQuickPickKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
		const isNext = event.key === "ArrowDown" || event.key === "ArrowRight";
		const isPrevious = event.key === "ArrowUp" || event.key === "ArrowLeft";
		if (!isNext && !isPrevious) {
			return;
		}
		const radios = Array.from(
			event.currentTarget.querySelectorAll<HTMLElement>('[role="radio"]'),
		);
		const current = radios.findIndex((radio) => radio === event.target);
		if (current === -1) {
			return;
		}
		const offset = isNext ? 1 : radios.length - 1;
		radios[(current + offset) % radios.length]?.focus();
		event.preventDefault();
	};

	const parsedFrom = parseClockTime(timeFields.from);
	const parsedTo = parseClockTime(timeFields.to);
	// Invalid text always blocks Apply, but the message waits for blur so
	// it does not flash while a partially typed time is still in flight.
	const fromTimeError =
		timeFields.fromTouched && parsedFrom === null ? INVALID_TIME_MESSAGE : null;
	const toTimeError =
		timeFields.toTouched && parsedTo === null ? INVALID_TIME_MESSAGE : null;

	const draftStart =
		selection?.from && parsedFrom
			? combineDateTime(selection.from, parsedFrom, timeFields.fromMeridiem)
			: null;
	// A range needs both boundaries; a single calendar click keeps Apply
	// disabled instead of silently committing a one-day range.
	const draftEnd =
		selection?.to && parsedTo
			? combineDateTime(selection.to, parsedTo, timeFields.toMeridiem)
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

	const selectedQuickPickIndex = customExpanded
		? quickPresets.length
		: Math.max(
				0,
				quickPresets.findIndex((preset) => preset.id === activePreset?.id),
			);

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
					{/* biome-ignore lint/a11y/useSemanticElements: native radio
					    inputs cannot host the check-icon rows this design needs. */}
					<div
						role="radiogroup"
						aria-label="Time range"
						onKeyDown={handleQuickPickKeyDown}
						className={cn(
							"flex flex-col gap-0.5 p-2 text-sm",
							customExpanded && "border-r border-border-default",
						)}
					>
						{quickPresets.map((preset, index) => (
							<QuickPickButton
								key={preset.id}
								label={preset.label}
								selected={!customExpanded && preset.id === activePreset?.id}
								tabIndex={index === selectedQuickPickIndex ? 0 : -1}
								onClick={() => handlePreset(preset)}
							/>
						))}
						<QuickPickButton
							label="Custom range"
							selected={customExpanded}
							tabIndex={selectedQuickPickIndex === quickPresets.length ? 0 : -1}
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
								disabled={{ after: currentTime }}
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
									onBlur={() =>
										setTimeFields((fields) => ({
											...fields,
											fromTouched: true,
										}))
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
									onBlur={() =>
										setTimeFields((fields) => ({ ...fields, toTouched: true }))
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
									<span
										role="alert"
										className="mr-auto text-sm text-content-destructive"
									>
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
	tabIndex: number;
	onClick: () => void;
}

const QuickPickButton: FC<QuickPickButtonProps> = ({
	label,
	selected,
	tabIndex,
	onClick,
}) => (
	// biome-ignore lint/a11y/useSemanticElements: see radiogroup note above.
	<button
		type="button"
		role="radio"
		aria-checked={selected}
		tabIndex={tabIndex}
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
	onBlur: () => void;
	onMeridiemChange: (meridiem: Meridiem) => void;
}

const TimeRow: FC<TimeRowProps> = ({
	id,
	label,
	time,
	meridiem,
	error,
	onTimeChange,
	onBlur,
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
				aria-describedby={error !== null ? `${id}-error` : undefined}
				className={cn(
					"h-8 w-28 tabular-nums",
					error !== null && "border-border-destructive",
				)}
				onChange={(event) => onTimeChange(event.target.value)}
				onBlur={onBlur}
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
			<span
				id={`${id}-error`}
				role="alert"
				className="pl-12 text-sm text-content-destructive"
			>
				{error}
			</span>
		)}
	</div>
);
