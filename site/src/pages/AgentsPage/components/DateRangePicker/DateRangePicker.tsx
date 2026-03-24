/**
 * A date-range picker composed from the project's Calendar, Popover, and
 * Button primitives. Replaces the legacy react-date-range based DateRange
 * component with one that matches the native design language.
 */
import { Button } from "components/Button/Button";
import { Calendar } from "../Calendar/Calendar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import dayjs from "dayjs";
import { CalendarIcon, MoveRightIcon } from "lucide-react";
import { type FC, useState } from "react";
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

	// Internal selection state kept separate from the committed value
	// so the user can freely adjust the range before applying.
	const [selection, setSelection] = useState<DayPickerDateRange | undefined>(
		() => ({
			from: value.startDate,
			to: value.endDate,
		}),
	);

	const commit = () => {
		if (selection?.from && selection?.to) {
			onChange(toBoundary(selection.from, selection.to));
		}
		setOpen(false);
	};

	const handlePreset = (preset: DateRangePreset) => {
		const { from, to } = preset.range();
		setSelection({ from, to });
		// Presets are a complete selection — commit immediately.
		onChange(toBoundary(from, to));
		setOpen(false);
	};

	const handleCalendarSelect = (range: DayPickerDateRange | undefined) => {
		if (!range) return;

		// react-day-picker resets the range on every click when both
		// endpoints are set (from is the clicked date, to is cleared).
		// Instead of forcing two clicks, adjust the nearest endpoint
		// so a single click is enough to tweak the range.
		if (range.from && !range.to && selection?.from && selection?.to) {
			const clicked = range.from.getTime();
			const distToStart = Math.abs(clicked - selection.from.getTime());
			const distToEnd = Math.abs(clicked - selection.to.getTime());

			if (distToStart <= distToEnd) {
				const newFrom = range.from;
				const newTo = newFrom <= selection.to ? selection.to : newFrom;
				setSelection({ from: newFrom, to: newTo });
			} else {
				const newTo = range.from;
				const newFrom = newTo >= selection.from ? selection.from : newTo;
				setSelection({ from: newFrom, to: newTo });
			}
			return;
		}

		setSelection(range);
	};

	// Sync local selection when the popover opens so it reflects the
	// latest committed value.
	const handleOpenChange = (next: boolean) => {
		if (next) {
			setSelection({ from: value.startDate, to: value.endDate });
		}
		setOpen(next);
	};

	const canApply =
		selection?.from &&
		selection?.to &&
		(selection.from.getTime() !== value.startDate.getTime() ||
			selection.to.getTime() !== value.endDate.getTime());

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
			<PopoverContent
				className="w-auto p-0 overflow-hidden"
				align="end"
				onOpenAutoFocus={(e) => e.preventDefault()}
			>
				<div className="flex">
					{/* Presets sidebar */}
					<div className="flex flex-col border-r border-border-default p-2 text-sm">
						{presets.map((preset) => (
							<button
								key={preset.label}
								type="button"
								onClick={() => handlePreset(preset)}
								className={cn(
									"cursor-pointer rounded-md border-none outline-none bg-transparent px-3 py-1.5 text-left text-sm",
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
								disabled={{ after: new Date() }}
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
