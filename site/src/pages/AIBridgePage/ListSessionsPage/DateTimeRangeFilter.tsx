import { CalendarClockIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { formatDateTime } from "#/utils/time";
import {
	fromLocalInputValue,
	type TimeRange,
	toLocalInputValue,
} from "./timeRange";

const TRIGGER_DATE_TIME_FORMAT = "MMM D, HH:mm";

interface DateTimeRangeFilterProps {
	value: TimeRange;
	onChange: (value: TimeRange) => void;
	/**
	 * True when the value is the in-memory default rather than an explicit
	 * user selection, so the trigger can advertise it as "Last 24 hours".
	 */
	isDefault: boolean;
	now?: Date;
}

export const DateTimeRangeFilter: FC<DateTimeRangeFilterProps> = ({
	value,
	onChange,
	isDefault,
	now,
}) => {
	const [open, setOpen] = useState(false);
	const currentTime = now ?? new Date();
	const startId = useId();
	const endId = useId();

	// Selection state is kept separate from the committed value so the
	// user can adjust the range freely before applying, and invalid or
	// unchanged selections never leak out.
	const [selection, setSelection] = useState<TimeRange>(value);

	const handleOpenChange = (next: boolean) => {
		if (next) {
			setSelection(value);
		}
		setOpen(next);
	};

	const applyDisabled =
		selection.startedAfter.getTime() >= selection.startedBefore.getTime() ||
		(selection.startedAfter.getTime() === value.startedAfter.getTime() &&
			selection.startedBefore.getTime() === value.startedBefore.getTime());

	const triggerLabel = isDefault
		? "Last 24 hours"
		: `${formatDateTime(value.startedAfter, TRIGGER_DATE_TIME_FORMAT)} → ${formatDateTime(value.startedBefore, TRIGGER_DATE_TIME_FORMAT)}`;

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button variant="outline" size="sm" aria-label="Filter by time range">
					<CalendarClockIcon className="size-4 text-content-secondary" />
					<span className="text-nowrap">{triggerLabel}</span>
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-auto p-4" align="end">
				<div className="flex flex-col gap-3">
					<div className="flex flex-col gap-1.5">
						<Label htmlFor={startId} className="text-content-secondary">
							From
						</Label>
						<Input
							id={startId}
							type="datetime-local"
							aria-label="Start of time range"
							step={60}
							max={toLocalInputValue(currentTime)}
							value={toLocalInputValue(selection.startedAfter)}
							onChange={(event) => {
								const parsed = fromLocalInputValue(event.target.value);
								if (parsed) {
									setSelection({
										...selection,
										startedAfter: parsed,
									});
								}
							}}
						/>
					</div>
					<div className="flex flex-col gap-1.5">
						<Label htmlFor={endId} className="text-content-secondary">
							To
						</Label>
						<Input
							id={endId}
							type="datetime-local"
							aria-label="End of time range"
							step={60}
							max={toLocalInputValue(currentTime)}
							value={toLocalInputValue(selection.startedBefore)}
							onChange={(event) => {
								const parsed = fromLocalInputValue(event.target.value);
								if (parsed) {
									setSelection({
										...selection,
										startedBefore: parsed,
									});
								}
							}}
						/>
					</div>
					<div className="flex items-center justify-end gap-2">
						<Button variant="subtle" size="sm" onClick={() => setOpen(false)}>
							Cancel
						</Button>
						<Button
							size="sm"
							disabled={applyDisabled}
							onClick={() => {
								onChange(selection);
								setOpen(false);
							}}
						>
							Apply
						</Button>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
};
