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
	/** Matches the SelectFilter trigger metrics in the filter row. */
	width?: number;
}

interface FieldState {
	text: string;
	touched: boolean;
}

const EXAMPLES = ["Now", "15:43", "2026-08-13 11:43"];

const NOW_TOLERANCE_MS = 60 * 1000;

// Boundaries at (or very near) the current moment read better as "now"
// than as a frozen timestamp when the popover reopens.
const toFieldText = (date: Date, now: Date): string =>
	Math.abs(date.getTime() - now.getTime()) < NOW_TOLERANCE_MS
		? "now"
		: formatTimeExpression(date);

const isNowExpression = (text: string): boolean =>
	text.trim().toLowerCase() === "now";

export const DateTimeRangeFilter: FC<DateTimeRangeFilterProps> = ({
	value,
	defaultValue,
	onChange,
	now,
	width = 200,
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
		touched: false,
	});
	const [toField, setToField] = useState<FieldState>({
		text: "",
		touched: false,
	});

	const handleOpenChange = useEffectEvent((next: boolean) => {
		if (next) {
			setFromField({
				text: toFieldText(value.startedAfter, currentTime),
				touched: false,
			});
			setToField({
				text: toFieldText(value.startedBefore, currentTime),
				touched: false,
			});
		}
		setOpen(next);
	});

	const parsedFrom = parseTimeExpression(fromField.text, currentTime);
	const parsedTo = parseTimeExpression(toField.text, currentTime);

	// Underspecified expressions resolve to absolute local timestamps when
	// the input loses focus, so the user sees exactly what will be applied.
	// "now" is left as-is because it is already unambiguous. Out-of-range
	// values are clamped against the other boundary so the committed range
	// is always valid.
	const normalizeFrom = useEffectEvent(() => {
		setFromField((field) => {
			if (isNowExpression(field.text)) {
				return field;
			}
			const parsed = parseTimeExpression(field.text, currentTime);
			if (!parsed) {
				return field;
			}
			const clamped =
				parsedTo !== null && parsed.getTime() >= parsedTo.getTime()
					? new Date(parsedTo.getTime() - 1000)
					: parsed;
			return { ...field, text: formatTimeExpression(clamped) };
		});
	});

	const normalizeTo = useEffectEvent(() => {
		setToField((field) => {
			if (isNowExpression(field.text)) {
				return field;
			}
			const parsed = parseTimeExpression(field.text, currentTime);
			if (!parsed) {
				return field;
			}
			const clamped =
				parsedFrom !== null && parsed.getTime() <= parsedFrom.getTime()
					? new Date(parsedFrom.getTime() + 1000)
					: parsed;
			return { ...field, text: formatTimeExpression(clamped) };
		});
	});

	const fromError =
		fromField.text !== "" && parsedFrom === null
			? "Enter a valid time, e.g. 2026-08-13 11:43"
			: null;
	const toError =
		toField.text !== "" && parsedTo === null
			? "Enter a valid time, e.g. 2026-08-13 11:43"
			: null;
	const rangeError =
		fromError === null &&
		toError === null &&
		parsedFrom !== null &&
		parsedTo !== null &&
		parsedFrom.getTime() >= parsedTo.getTime()
			? "From must be before To"
			: null;

	// Apply is only useful when something actually changed; untouched
	// fields resolve back to the committed range.
	const modified = fromField.touched || toField.touched;
	const applyDisabled =
		!modified || fromError !== null || toError !== null || rangeError !== null;

	const triggerLabel = isDefault
		? "Last 24 hours"
		: formatTriggerLabel(value, currentTime);

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					variant="outline"
					aria-label="Filter by time range"
					className="grow justify-start"
					style={{ flexBasis: width }}
				>
					<CalendarIcon className="size-4 shrink-0 text-content-secondary" />
					<span className="truncate text-left">{triggerLabel}</span>
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
							aria-invalid={fromError !== null}
							placeholder="now"
							className={cn(fromError !== null && "border-border-destructive")}
							value={fromField.text}
							onChange={(event) => {
								const text = event.target.value;
								setFromField({ text, touched: true });
							}}
							onBlur={normalizeFrom}
						/>
						{fromError !== null && (
							<span className="text-sm text-content-destructive">
								{fromError}
							</span>
						)}
					</div>
					<div className="flex flex-col gap-1.5">
						<Label htmlFor="time-range-to" className="text-content-primary">
							To
						</Label>
						<Input
							id="time-range-to"
							aria-label="End of time range"
							aria-invalid={toError !== null}
							placeholder="now"
							className={cn(toError !== null && "border-border-destructive")}
							value={toField.text}
							onChange={(event) => {
								const text = event.target.value;
								setToField({ text, touched: true });
							}}
							onBlur={normalizeTo}
						/>
						{toError !== null && (
							<span className="text-sm text-content-destructive">
								{toError}
							</span>
						)}
					</div>
					<div className="flex flex-col items-end gap-2">
						{rangeError !== null && (
							<span className="text-sm text-content-destructive">
								{rangeError}
							</span>
						)}
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
