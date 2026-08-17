import { CalendarIcon } from "lucide-react";
import { type FC, useEffectEvent, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";
import {
	formatTimeExpression,
	formatTriggerLabel,
	parseTimeExpression,
	type TimeRange,
} from "./timeRange";

interface DateTimeRangeFilterProps {
	value: TimeRange;
	/**
	 * The range the page falls back to when no explicit filter is set.
	 * The trigger labels it as "Last 24 hours" while the value equals it.
	 */
	defaultValue: TimeRange;
	onChange: (value: TimeRange) => void;
	now?: Date;
}

interface FieldState {
	text: string;
	error: boolean;
}

const EXAMPLES = ["Now", "15:43", "2026-08-13 11:43"];

const NOW_TOLERANCE_MS = 60 * 1000;

// Boundaries at (or very near) the current moment read better as "now"
// than as a frozen timestamp when the popover reopens.
const toFieldText = (date: Date, now: Date): string =>
	Math.abs(date.getTime() - now.getTime()) < NOW_TOLERANCE_MS
		? "now"
		: formatTimeExpression(date);

export const DateTimeRangeFilter: FC<DateTimeRangeFilterProps> = ({
	value,
	defaultValue,
	onChange,
	now,
}) => {
	const [open, setOpen] = useState(false);
	const currentTime = now ?? new Date();
	const isDefault =
		value.startedAfter.getTime() === defaultValue.startedAfter.getTime() &&
		value.startedBefore.getTime() === defaultValue.startedBefore.getTime();

	// Text state is kept separate from the committed value so the user can
	// adjust the expressions freely before applying; invalid text never
	// leaks out.
	const [fromField, setFromField] = useState<FieldState>({
		text: "",
		error: false,
	});
	const [toField, setToField] = useState<FieldState>({
		text: "",
		error: false,
	});

	const handleOpenChange = useEffectEvent((next: boolean) => {
		if (next) {
			setFromField({
				text: toFieldText(value.startedAfter, currentTime),
				error: false,
			});
			setToField({
				text: toFieldText(value.startedBefore, currentTime),
				error: false,
			});
		}
		setOpen(next);
	});

	const parsedFrom = parseTimeExpression(fromField.text, currentTime);
	const parsedTo = parseTimeExpression(toField.text, currentTime);
	// "now" resolves to the current moment on every render, so compare with
	// the same tolerance used when prefilling the inputs.
	const unchanged =
		parsedFrom !== null &&
		parsedTo !== null &&
		Math.abs(parsedFrom.getTime() - value.startedAfter.getTime()) <
			NOW_TOLERANCE_MS &&
		Math.abs(parsedTo.getTime() - value.startedBefore.getTime()) <
			NOW_TOLERANCE_MS;
	const applyDisabled =
		fromField.error ||
		toField.error ||
		parsedFrom === null ||
		parsedTo === null ||
		unchanged ||
		parsedFrom.getTime() >= parsedTo.getTime();

	const triggerLabel = isDefault
		? "Last 24 hours"
		: formatTriggerLabel(value, currentTime);

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button variant="outline" size="sm" aria-label="Filter by time range">
					<CalendarIcon className="size-4 text-content-secondary" />
					<span className="text-nowrap">{triggerLabel}</span>
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-80 p-0" align="end">
				<div className="flex flex-col gap-3 p-4">
					<div className="flex flex-col gap-1.5">
						<Label htmlFor="time-range-from" className="text-content-primary">
							From
						</Label>
						<Input
							id="time-range-from"
							aria-label="Start of time range"
							aria-invalid={fromField.error}
							placeholder="now"
							className={cn(fromField.error && "border-border-destructive")}
							value={fromField.text}
							onChange={(event) => {
								const text = event.target.value;
								setFromField({
									text,
									error:
										text !== "" &&
										parseTimeExpression(text, currentTime) === null,
								});
							}}
						/>
					</div>
					<div className="flex flex-col gap-1.5">
						<Label htmlFor="time-range-to" className="text-content-primary">
							To
						</Label>
						<Input
							id="time-range-to"
							aria-label="End of time range"
							aria-invalid={toField.error}
							placeholder="now"
							className={cn(toField.error && "border-border-destructive")}
							value={toField.text}
							onChange={(event) => {
								const text = event.target.value;
								setToField({
									text,
									error:
										text !== "" &&
										parseTimeExpression(text, currentTime) === null,
								});
							}}
						/>
					</div>
					<div className="flex items-center justify-end">
						<Button
							size="sm"
							disabled={applyDisabled}
							onClick={() => {
								if (parsedFrom && parsedTo) {
									onChange({
										startedAfter: parsedFrom,
										startedBefore: parsedTo,
									});
								}
								setOpen(false);
							}}
						>
							Apply
						</Button>
					</div>
				</div>
				<div className="flex flex-col gap-2 border-t border-border-default p-4 text-sm text-content-secondary">
					<span className="font-semibold text-content-primary">Examples:</span>
					<span>{EXAMPLES.join(" | ")}</span>
					<span>Defaults to midnight if no time is provided.</span>
					<span>Defaults to current day if no date is provided.</span>
				</div>
			</PopoverContent>
		</Popover>
	);
};
