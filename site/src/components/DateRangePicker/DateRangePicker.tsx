/**
 * A date-range picker composed from the project's Calendar, Popover, and
 * Button primitives. Replaces the legacy react-date-range based DateRange
 * component with one that matches the native design language.
 */
import { Button } from "components/Button/Button";
import { Calendar } from "components/Calendar/Calendar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import dayjs from "dayjs";
import { CalendarIcon, MoveRightIcon } from "lucide-react";
import { type FC, useCallback, useState } from "react";
import type { DateRange as DayPickerDateRange } from "react-day-picker";
import { cn } from "utils/cn";

export type DateRangeValue = {
	startDate: Date;
	endDate: Date;
};

interface DateRangePreset {
	label: string;
	range: () => { from: Date; to: Date };
}

const defaultPresets: DateRangePreset[] = [
	{
		label: "Today",
		range: () => ({ from: new Date(), to: new Date() }),
	},
	{
		label: "Yesterday",
		range: () => {
			const d = dayjs().subtract(1, "day").toDate();
			return { from: d, to: d };
		},
	},
	{
		label: "Last 7 days",
		range: () => ({
			from: dayjs().subtract(6, "day").toDate(),
			to: new Date(),
		}),
	},
	{
		label: "Last 14 days",
		range: () => ({
			from: dayjs().subtract(13, "day").toDate(),
			to: new Date(),
		}),
	},
	{
		label: "Last 30 days",
		range: () => ({
			from: dayjs().subtract(29, "day").toDate(),
			to: new Date(),
		}),
	},
];

interface DateRangePickerProps {
	value: DateRangeValue;
	onChange: (value: DateRangeValue) => void;
	presets?: DateRangePreset[];
}

/**
 * Normalise a calendar selection into the API-friendly boundary format
 * that the old component produced: startDate at midnight, endDate either
 * rounded up to the next hour (if it falls on today) or to the start of
 * the following day.
 */
function toBoundary(from: Date, to: Date): DateRangeValue {
	const start = dayjs(from).startOf("day").toDate();
	const end = dayjs(to).isSame(dayjs(), "day")
		? dayjs().startOf("hour").add(1, "hour").toDate()
		: dayjs(to).startOf("day").add(1, "day").toDate();
	return { startDate: start, endDate: end };
}

export const DateRangePicker: FC<DateRangePickerProps> = ({
	value,
	onChange,
	presets = defaultPresets,
}) => {
	const [open, setOpen] = useState(false);

	// Internal selection state for the two-month calendar. Kept separate
	// from the committed `value` so that a single click doesn't immediately
	// fire `onChange`.
	const [selection, setSelection] = useState<DayPickerDateRange | undefined>(
		() => ({
			from: value.startDate,
			to: value.endDate,
		}),
	);

	const commit = useCallback(
		(from: Date, to: Date) => {
			onChange(toBoundary(from, to));
			setOpen(false);
		},
		[onChange],
	);

	const handlePreset = useCallback(
		(preset: DateRangePreset) => {
			const { from, to } = preset.range();
			setSelection({ from, to });
			commit(from, to);
		},
		[commit],
	);

	const handleCalendarSelect = useCallback(
		(range: DayPickerDateRange | undefined) => {
			if (!range) return;
			setSelection(range);

			// react-day-picker fires onChange on every click. A complete
			// range (both `from` and `to` present and different) means the
			// user finished their two-click selection.
			if (range.from && range.to && range.from !== range.to) {
				commit(range.from, range.to);
			}
		},
		[commit],
	);

	// Sync local selection when the popover opens so it reflects the
	// latest committed value.
	const handleOpenChange = useCallback(
		(next: boolean) => {
			if (next) {
				setSelection({ from: value.startDate, to: value.endDate });
			}
			setOpen(next);
		},
		[value],
	);

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button variant="outline" size="sm">
					<CalendarIcon className="size-4 text-content-secondary" />
					<span>{dayjs(value.startDate).format("MMM D, YYYY")}</span>
					<MoveRightIcon className="size-3.5 text-content-secondary" />
					<span>{dayjs(value.endDate).format("MMM D, YYYY")}</span>
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-auto p-0 overflow-hidden" align="end">
				<div className="flex">
					{/* Presets sidebar */}
					<div className="flex flex-col border-r border-solid border-border-default p-2 text-sm">
						{presets.map((preset) => (
							<button
								key={preset.label}
								type="button"
								onClick={() => handlePreset(preset)}
								className={cn(
									"cursor-pointer rounded-md border-0 bg-transparent px-3 py-1.5 text-left text-sm",
									"text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
									"transition-colors whitespace-nowrap",
								)}
							>
								{preset.label}
							</button>
						))}
					</div>

					{/* Two-month calendar */}
					<div className="p-2">
						<Calendar
							mode="range"
							selected={selection}
							onSelect={handleCalendarSelect}
							numberOfMonths={2}
							disabled={{ after: new Date() }}
						/>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
};
