/**
 * A date-and-time range picker with quick picks. The calendar and time fields stay
 * hidden until "Custom range" is chosen.
 */

import { CalendarIcon, CheckIcon } from "lucide-react";
import { type FC, type KeyboardEvent, useEffect, useId, useState } from "react";
import type { DateRange as DayPickerDateRange } from "react-day-picker";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
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
// How long the floating error stays visible before fading, mirroring
// toast behavior. The invalid field styling and disabled Apply remain
// until the input is corrected.
const ERROR_DISMISS_TIMEOUT_MS = 5_000;

interface TimeFieldsState {
	from: string;
	fromMeridiem: Meridiem;
	fromTouched: boolean;
	to: string;
	toMeridiem: Meridiem;
	toTouched: boolean;
}

// From defaults to the start of the day and To to the end, so any
// day-only selection spans the full final day.
const defaultTimeFields = (): TimeFieldsState => ({
	from: "12:00:00",
	fromMeridiem: "AM",
	fromTouched: false,
	to: "11:59:59",
	toMeridiem: "PM",
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
	const [timeFields, setTimeFields] =
		useState<TimeFieldsState>(defaultTimeFields);
	const fromTimeId = useId();
	const toTimeId = useId();
	const errorId = useId();

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
				setTimeFields(defaultTimeFields());
			}
		}
		setOpen(next);
	};

	const handlePreset = (preset: QuickPreset) => {
		const { start, end } = preset.range(currentTime);
		onChange({ start, end, preset: preset.id });
		setOpen(false);
	};

	// Roving focus for the quick-pick radiogroup.
	const handleQuickPickKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
		const isNext = event.key === "ArrowDown" || event.key === "ArrowRight";
		const isPrevious = event.key === "ArrowUp" || event.key === "ArrowLeft";
		if (!isNext && !isPrevious) {
			return;
		}
		const radios = Array.from(
			event.currentTarget.querySelectorAll<HTMLElement>('[role="radio"]'),
		);
		const current =
			event.target instanceof HTMLElement ? radios.indexOf(event.target) : -1;
		if (current === -1) {
			return;
		}
		const offset = isNext ? 1 : radios.length - 1;
		radios[(current + offset) % radios.length]?.focus();
		event.preventDefault();
	};

	const parsedFrom = parseClockTime(timeFields.from);
	const parsedTo = parseClockTime(timeFields.to);
	const fromTimeError =
		timeFields.fromTouched && parsedFrom === null ? INVALID_TIME_MESSAGE : null;
	const toTimeError =
		timeFields.toTouched && parsedTo === null ? INVALID_TIME_MESSAGE : null;

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

	const errorMessage = fromTimeError ?? toTimeError ?? rangeError;

	// Toast-style dismissal: each message hides after a timeout but is
	// remembered so it does not reappear until the error actually changes.
	const [dismissedMessage, setDismissedMessage] = useState<string | null>(null);
	useEffect(() => {
		if (errorMessage === null) {
			setDismissedMessage(null);
			return;
		}
		const timer = setTimeout(
			() => setDismissedMessage(errorMessage),
			ERROR_DISMISS_TIMEOUT_MS,
		);
		return () => clearTimeout(timer);
	}, [errorMessage]);
	const visibleError =
		errorMessage !== null && errorMessage !== dismissedMessage
			? errorMessage
			: null;

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
				{/* The trigger mirrors ComboboxButton (gap-2, pr-1.5, icons
				    inheriting the button's primary text color) so it matches
				    the other filter dropdowns. The calendar icon sizing is
				    marked important because the Button's `[&>svg]` rules would
				    otherwise win on specificity and inflate it at larger button
				    sizes; it matches the search field icon (size-icon-sm) with
				    the same 8px icon-to-text spacing via gap-2. The calendar
				    glyph is denser than the magnifier, so its stroke is
				    slightly thinner to read as the same optical weight. */}
				<Button variant="outline" size={size} className="group gap-2 pr-1.5">
					<CalendarIcon strokeWidth={1.75} className="!size-icon-sm !p-0" />
					<span>{triggerLabel}</span>
					<ChevronDownIcon className="size-icon-sm" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				className="w-auto p-0 overflow-x-hidden overflow-y-auto"
				align="end"
				onOpenAutoFocus={(e) => e.preventDefault()}
			>
				<div className="flex">
					{/* Quick picks. This list is the entire dropdown until the
					    user expands the custom range panel. */}
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
							<div className="relative">
								<Calendar
									mode="range"
									selected={selection}
									onSelect={setSelection}
									defaultMonth={
										value.preset === undefined ? value.start : currentTime
									}
									disabled={{ after: currentTime }}
									endMonth={currentTime}
									today={currentTime}
								/>
								{visibleError !== null && (
									<div
										id={errorId}
										role="alert"
										className="pointer-events-none absolute inset-x-3 bottom-3 rounded-md border border-solid border-border-destructive bg-surface-primary px-3 py-2 text-sm text-content-destructive shadow-md"
									>
										{visibleError}
									</div>
								)}
							</div>

							{/* From/To time fields */}
							<div className="flex flex-col gap-2 border-t border-border-default px-3 py-3">
								<TimeRow
									id={fromTimeId}
									label="From"
									time={timeFields.from}
									meridiem={timeFields.fromMeridiem}
									invalid={fromTimeError !== null}
									describedBy={
										fromTimeError !== null && visibleError === fromTimeError
											? errorId
											: undefined
									}
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
									invalid={toTimeError !== null}
									describedBy={
										toTimeError !== null && visibleError === toTimeError
											? errorId
											: undefined
									}
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
	invalid: boolean;
	describedBy: string | undefined;
	onTimeChange: (time: string) => void;
	onBlur: () => void;
	onMeridiemChange: (meridiem: Meridiem) => void;
}

const TimeRow: FC<TimeRowProps> = ({
	id,
	label,
	time,
	meridiem,
	invalid,
	describedBy,
	onTimeChange,
	onBlur,
	onMeridiemChange,
}) => (
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
			aria-invalid={invalid}
			aria-describedby={describedBy}
			className={cn(
				"h-8 w-28 tabular-nums",
				invalid && "border-border-destructive",
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
);
