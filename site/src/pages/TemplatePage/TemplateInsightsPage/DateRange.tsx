import { Button } from "components/Button/Button";
import { Calendar } from "components/Calendar/Calendar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import dayjs from "dayjs";
import { ArrowRightIcon, CalendarIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import type { DateRange as DayPickerDateRange } from "react-day-picker";

export type DateRangeValue = {
	startDate: Date;
	endDate: Date;
};

interface DateRangeProps {
	value: DateRangeValue;
	onChange: (value: DateRangeValue) => void;
}

const presets = [
	{ label: "Today", days: 0 },
	{ label: "Yesterday", days: 1 },
	{ label: "Last 7 days", days: 6 },
	{ label: "Last 14 days", days: 13 },
	{ label: "Last 30 days", days: 29 },
] as const;

export const DateRange: FC<DateRangeProps> = ({ value, onChange }) => {
	const selectionRef = useRef<"idle" | "selecting">("idle");
	const [selected, setSelected] = useState<DayPickerDateRange | undefined>({
		from: value.startDate,
		to: dayjs(value.endDate).subtract(1, "millisecond").toDate(),
	});
	const [open, setOpen] = useState(false);

	const handleSelect = (range: DayPickerDateRange | undefined) => {
		setSelected(range);

		if (!range?.from) {
			selectionRef.current = "idle";
			return;
		}

		// First click starts the range -- don't close yet.
		if (selectionRef.current === "idle") {
			selectionRef.current = "selecting";
			return;
		}

		// Second click completes the range.
		selectionRef.current = "idle";
		if (range.from && range.to) {
			const endDate = dayjs(range.to).isSame(dayjs(), "day")
				? dayjs().startOf("hour").add(1, "hour").toDate()
				: dayjs(range.to).startOf("day").add(1, "day").toDate();
			onChange({
				startDate: dayjs(range.from).startOf("day").toDate(),
				endDate,
			});
		}
	};

	const handlePreset = (days: number) => {
		const from = dayjs().subtract(days, "day").toDate();
		const to = new Date();
		setSelected({ from, to });
		onChange({
			startDate: dayjs(from).startOf("day").toDate(),
			endDate: dayjs().startOf("hour").add(1, "hour").toDate(),
		});
	};

	return (
		<Popover
			open={open}
			onOpenChange={(next) => {
				setOpen(next);
				// Reset selection state when opening.
				if (next) {
					selectionRef.current = "idle";
					setSelected({
						from: value.startDate,
						to: dayjs(value.endDate).subtract(1, "millisecond").toDate(),
					});
				}
			}}
		>
			<PopoverTrigger asChild>
				<Button variant="outline">
					<CalendarIcon className="!size-icon-sm" />
					<span>{dayjs(value.startDate).format("MMM D, YYYY")}</span>
					<ArrowRightIcon className="!size-icon-sm" />
					<span>
						{dayjs(value.endDate)
							.subtract(1, "millisecond")
							.format("MMM D, YYYY")}
					</span>
				</Button>
			</PopoverTrigger>
			<PopoverContent
				className="w-auto p-0 flex overflow-visible"
				align="start"
			>
				<div className="flex flex-col gap-1 border-r border-border p-3 text-sm w-32">
					{presets.map((preset) => (
						<Button
							key={preset.label}
							variant="subtle"
							size="sm"
							className="justify-start"
							onClick={() => handlePreset(preset.days)}
						>
							{preset.label}
						</Button>
					))}
				</div>
				<Calendar
					mode="range"
					captionLayout="dropdown"
					defaultMonth={
						new Date(new Date().getFullYear(), new Date().getMonth() - 1)
					}
					selected={selected}
					onSelect={handleSelect}
					numberOfMonths={2}
					disabled={{ after: new Date() }}
				/>
			</PopoverContent>
		</Popover>
	);
};
