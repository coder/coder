/**
 * A date-range picker composed from the project's Calendar, Popover, and
 * Button primitives. Replaces the legacy react-date-range based DateRange
 * component with one that matches the native design language.
 */

import { cn } from "cn";
import dayjs from "dayjs";
import { CalendarIcon, MoveRightIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { DateRange as DayPickerDateRange } from "react-day-picker";
import { Button, type ButtonProps } from "#/components/Button/Button";
import { Calendar } from "#/components/Calendar/Calendar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";

export type DateRangeValue = {
	startDate: Date;
	endDate: Date;
};

type DateRangePreset = {
	label: string;
	range: () => { from: Date; to: Date };
};

const buildDefaultPresets = (now?: Date): DateRangePreset[] => {
	const getCurrentTime = () => dayjs(now ?? new Date());
	return [
		{
			label: "Today",
			range: () => {
				const currentTime = getCurrentTime();
				return { from: currentTime.toDate(), to: currentTime.toDate() };
			},
		},
		{
			label: "Yesterday",
			range: () => {
				const d = getCurrentTime().subtract(1, "day").toDate();
				return { from: d, to: d };
			},
		},
		{
			label: "Last 7 days",
			range: () => {
				const currentTime = getCurrentTime();
				return {
					from: currentTime.subtract(6, "day").toDate(),
					to: currentTime.toDate(),
				};
			},
		},
		{
			label: "Last 14 days",
			range: () => {
				const currentTime = getCurrentTime();
				return {
					from: currentTime.subtract(13, "day").toDate(),
					to: currentTime.toDate(),
				};
			},
		},
		{
			label: "Last 30 days",
			range: () => {
				const currentTime = getCurrentTime();
				return {
					from: currentTime.subtract(29, "day").toDate(),
					to: currentTime.toDate(),
				};
			},
		},
	];
};

type DateRangePickerProps = {
	value: DateRangeValue;
	onChange: (value: DateRangeValue) => void;
	now?: Date;
	presets?: DateRangePreset[];
	size?: ButtonProps["size"];
	/**
	 * Longest range the user can select, in inclusive days. Presets that
	 * would exceed it are hidden.
	 */
	maxDays?: number;
	/** Earliest selectable day. Presets that would start before it are hidden. */
	minDate?: Date;
};

/**
 * Normalise a calendar selection into the API-friendly boundary format
 * that the old component produced: startDate at midnight, endDate either
 * rounded up to the next hour (if it falls on today) or to the start of
 * the following day.
 */
function toBoundary(from: Date, to: Date, now: Date): DateRangeValue {
	const currentTime = dayjs(now);
	const start = dayjs(from).startOf("day").toDate();
	const end = dayjs(to).isSame(currentTime, "day")
		? currentTime.startOf("hour").add(1, "hour").toDate()
		: dayjs(to).startOf("day").add(1, "day").toDate();
	return { startDate: start, endDate: end };
}

function firstDayOnOrAfter(date: Date): Date {
	const day = dayjs(date).startOf("day");
	return (day.isSame(date) ? day : day.add(1, "day")).toDate();
}

/**
 * Reverse the boundary normalization so the calendar highlights the
 * inclusive end date the user originally selected, not the exclusive
 * API boundary. Midnight boundaries get shifted back by one day;
 * sub-day boundaries (today's rounded-up hour) stay on the same day.
 */
function fromBoundary(value: DateRangeValue): DayPickerDateRange {
	const from = dayjs(value.startDate).startOf("day").toDate();
	const endDayjs = dayjs(value.endDate);
	const to = endDayjs.isSame(endDayjs.startOf("day"))
		? endDayjs.subtract(1, "day").toDate()
		: endDayjs.toDate();
	return { from, to };
}

export const DateRangePicker: FC<DateRangePickerProps> = ({
	value,
	onChange,
	now,
	presets,
	size = "sm",
	maxDays,
	minDate,
}) => {
	const [open, setOpen] = useState(false);
	const currentTime = now ?? new Date();

	// maxDays counts local calendar days, but the committed boundary is an
	// interval that APIs bound by exact duration, and a maximal range of local
	// days that crosses a fall daylight-saving transition runs long by the
	// shift (an hour, or half an hour in some zones). Allow one day less from
	// such a start rather than trimming the committed range.
	const selectableDaysFrom = (from: Date): number | undefined => {
		if (maxDays === undefined) {
			return undefined;
		}
		const lastDay = dayjs(from)
			.add(maxDays - 1, "day")
			.toDate();
		const { startDate, endDate } = toBoundary(from, lastDay, currentTime);
		return dayjs(endDate).diff(startDate, "hour", true) > maxDays * 24
			? maxDays - 1
			: maxDays;
	};
	const fitsMaxDays = (from: Date, to: Date): boolean => {
		const limit = selectableDaysFrom(from);
		return limit === undefined || dayjs(to).diff(from, "day") + 1 <= limit;
	};

	// Committed starts are local midnights, so a cutoff inside a day excludes
	// that whole day rather than emitting a start before the cutoff.
	const firstSelectableDay =
		minDate === undefined ? undefined : firstDayOnOrAfter(minDate);
	const resolvedPresets = (presets ?? buildDefaultPresets(now)).filter(
		(preset) => {
			const { from, to } = preset.range();
			return (
				(firstSelectableDay === undefined || from >= firstSelectableDay) &&
				fitsMaxDays(from, to)
			);
		},
	);

	// Internal selection state kept separate from the committed value
	// so the user can freely adjust the range before applying. This
	// uses raw calendar dates (inclusive), not the API boundary format.
	const [selection, setSelection] = useState<DayPickerDateRange | undefined>(
		() => fromBoundary(value),
	);

	const commit = () => {
		if (selection?.from && selection?.to) {
			onChange(toBoundary(selection.from, selection.to, now ?? new Date()));
		}
		setOpen(false);
	};

	const handlePreset = (preset: DateRangePreset) => {
		const { from, to } = preset.range();
		setSelection({ from, to });
		// Presets are a complete selection — commit immediately.
		onChange(toBoundary(from, to, now ?? new Date()));
		setOpen(false);
	};

	// The limit depends on the candidate's own start, which react-day-picker's
	// max cannot express (and it treats a max of zero as unlimited), so the
	// candidate is checked here and, like an over-long react-day-picker range,
	// restarted from the clicked day.
	const handleCalendarSelect = (
		range: DayPickerDateRange | undefined,
		triggerDate: Date,
	) => {
		if (!range) return;
		if (range.from && range.to && !fitsMaxDays(range.from, range.to)) {
			setSelection({
				from: triggerDate,
				to: selectableDaysFrom(triggerDate) === 1 ? triggerDate : undefined,
			});
			return;
		}
		setSelection(range);
	};

	// Sync local selection when the popover opens so it reflects the
	// latest committed value. Reverse the boundary normalization so
	// the calendar highlights the correct inclusive dates.
	const handleOpenChange = (next: boolean) => {
		if (next) {
			setSelection(fromBoundary(value));
		}
		setOpen(next);
	};

	// Compare in the same coordinate space (raw calendar dates) so
	// re-selecting the identical range doesn't enable Apply.
	const committed = fromBoundary(value);
	const canApply =
		selection?.from &&
		selection?.to &&
		(selection.from.getTime() !== committed.from?.getTime() ||
			selection.to.getTime() !== committed.to?.getTime());

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button variant="outline" size={size}>
					<CalendarIcon className="size-4 text-content-secondary" />
					<span>{dayjs(committed.from).format("MMM D, YYYY")}</span>
					<MoveRightIcon className="size-3.5 text-content-secondary" />
					<span>{dayjs(committed.to).format("MMM D, YYYY")}</span>
				</Button>
			</PopoverTrigger>
			<PopoverContent
				className="w-auto p-0 overflow-x-hidden overflow-y-auto"
				align="end"
				onOpenAutoFocus={(e) => e.preventDefault()}
			>
				<div className="flex">
					{/* Presets sidebar */}
					<div className="flex flex-col border-r border-border-default p-2 text-sm">
						{resolvedPresets.map((preset) => (
							<button
								key={preset.label}
								type="button"
								onClick={() => handlePreset(preset)}
								className={cn(
									"cursor-pointer rounded-md border-none outline-hidden bg-transparent px-3 py-1.5 text-left text-sm",
									"text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
									"focus-visible:ring-2 focus-visible:ring-content-link",
									"transition-colors whitespace-nowrap",
								)}
							>
								{preset.label}
							</button>
						))}
					</div>

					{/* Calendar + footer */}
					<div className="flex flex-col">
						{/* Selected range display */}
						<div className="flex items-center gap-2 border-b border-border-default px-4 py-2 text-sm">
							<span
								className={cn(
									"rounded-md px-2 py-1 tabular-nums",
									selection?.from
										? "bg-surface-secondary text-content-primary"
										: "text-content-secondary",
								)}
							>
								{selection?.from
									? dayjs(selection.from).format("MMM D, YYYY")
									: "Start date"}
							</span>
							<MoveRightIcon className="size-3.5 text-content-secondary" />
							<span
								className={cn(
									"rounded-md px-2 py-1 tabular-nums",
									selection?.to
										? "bg-surface-secondary text-content-primary"
										: "text-content-secondary",
								)}
							>
								{selection?.to
									? dayjs(selection.to).format("MMM D, YYYY")
									: "End date"}
							</span>
						</div>

						{/* Two-month calendar */}
						<div className="p-2">
							<Calendar
								mode="range"
								selected={selection}
								onSelect={handleCalendarSelect}
								numberOfMonths={2}
								excludeDisabled
								disabled={
									firstSelectableDay === undefined
										? { after: currentTime }
										: [{ before: firstSelectableDay }, { after: currentTime }]
								}
								today={currentTime}
							/>
						</div>

						{/* Apply footer */}
						<div className="flex items-center justify-end gap-2 border-t border-border-default px-4 py-2">
							<Button variant="subtle" size="sm" onClick={() => setOpen(false)}>
								Cancel
							</Button>
							<Button size="sm" onClick={commit} disabled={!canApply}>
								Apply
							</Button>
						</div>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
};
