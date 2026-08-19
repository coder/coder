import dayjs from "dayjs";
import { CalendarIcon } from "lucide-react";
import { type FC, useEffectEvent, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";
import { formatDateTime } from "#/utils/time";
import {
	type FullTimeRange,
	formatTriggerLabel,
	isLiveNow,
	isNowExpression,
	parseTimeExpression,
	type TimeRange,
} from "./timeRange";

interface DateTimeRangeFilterProps {
	value: TimeRange;
	/**
	 * The range the page falls back to when no explicit filter is set.
	 * The trigger labels it as "Last 24 hours" while the value equals it.
	 */
	defaultValue: FullTimeRange;
	onChange: (value: FullTimeRange) => void;
	now?: Date;
	/** Matches the SelectFilter trigger metrics in the filter row. */
	width?: number;
}

interface FieldState {
	text: string;
	touched: boolean;
}

const INVALID_TIME_MESSAGE = "Enter a valid time, e.g. 2026-08-13 11:43";

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
		value.startedAfter !== undefined &&
		value.startedBefore !== undefined &&
		value.startedAfter.getTime() === defaultValue.startedAfter?.getTime() &&
		value.startedBefore.getTime() === defaultValue.startedBefore?.getTime();

	// Text state is kept separate from the committed value so the user can
	// adjust the expressions freely before applying; invalid text never
	// leaks out. If this control grows more fields or validation rules,
	// consider moving to formik and yup instead of hand-rolling state.
	const [fromField, setFromField] = useState<FieldState>({
		text: "",
		touched: false,
	});
	const [toField, setToField] = useState<FieldState>({
		text: "",
		touched: false,
	});

	// Hidden datetime-local inputs back the calendar buttons, so the free
	// text grammar ("now", clock-only, ISO) and the native picker coexist.
	const fromPickerRef = useRef<HTMLInputElement>(null);
	const toPickerRef = useRef<HTMLInputElement>(null);

	const openPicker = (input: HTMLInputElement | null, seed: string) => {
		if (!input) {
			return;
		}
		// Seed the picker from the current text so it opens on the right
		// moment; fall back to now for empty or non-absolute expressions. The
		// native picker speaks "YYYY-MM-DDTHH:mm" in browser-local time.
		const parsed = parseTimeExpression(seed, currentTime);
		input.value = dayjs(parsed ?? currentTime).format("YYYY-MM-DDTHH:mm");
		input.showPicker();
	};

	const pickFrom = useEffectEvent((value: string) => {
		const parsed = parseTimeExpression(value, currentTime);
		if (parsed !== null) {
			setFromField({ text: formatDateTime(parsed), touched: true });
		}
	});

	const pickTo = useEffectEvent((value: string) => {
		const parsed = parseTimeExpression(value, currentTime);
		if (parsed !== null) {
			setToField({ text: formatDateTime(parsed), touched: true });
		}
	});

	const handleOpenChange = useEffectEvent((next: boolean) => {
		if (next) {
			// Boundaries at (or very near) the current moment read better
			// as "now" than as a frozen timestamp when the popover reopens.
			// A missing bound stays empty so the popover reflects the query.
			const toFieldText = (date: Date | undefined): string => {
				if (!date) {
					return "";
				}
				return isLiveNow(date, currentTime) ? "now" : formatDateTime(date);
			};
			setFromField({
				text: toFieldText(value.startedAfter),
				touched: false,
			});
			setToField({
				text: toFieldText(value.startedBefore),
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
	// values clamp against the other boundary so the committed range is
	// always valid.
	const normalizeFrom = useEffectEvent(() => {
		setFromField((current) => {
			if (isNowExpression(current.text) || parsedFrom === null) {
				return current;
			}
			const clamped =
				parsedTo !== null && parsedFrom.getTime() >= parsedTo.getTime()
					? new Date(parsedTo.getTime() - 1000)
					: parsedFrom;
			return { ...current, text: formatDateTime(clamped) };
		});
	});

	const normalizeTo = useEffectEvent(() => {
		setToField((current) => {
			if (isNowExpression(current.text) || parsedTo === null) {
				return current;
			}
			const clamped =
				parsedFrom !== null && parsedTo.getTime() <= parsedFrom.getTime()
					? new Date(parsedFrom.getTime() + 1000)
					: parsedTo;
			return { ...current, text: formatDateTime(clamped) };
		});
	});

	const fromError =
		fromField.text !== "" && parsedFrom === null ? INVALID_TIME_MESSAGE : null;
	const toError =
		toField.text !== "" && parsedTo === null ? INVALID_TIME_MESSAGE : null;
	const rangeError =
		fromError === null &&
		toError === null &&
		parsedFrom !== null &&
		parsedTo !== null &&
		parsedFrom.getTime() >= parsedTo.getTime()
			? "From must be before To"
			: null;

	// Apply is only useful when something actually changed; untouched
	// fields resolve back to the committed range. Both bounds are required
	// so the popover always commits a full range; a one-sided range is a
	// deliberate free-text-only escape hatch.
	const applyDisabled =
		!(fromField.touched || toField.touched) ||
		fromField.text === "" ||
		toField.text === "" ||
		fromError !== null ||
		toError !== null ||
		rangeError !== null;

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
						<div className="relative">
							<Input
								id="time-range-from"
								aria-label="Start of time range"
								aria-invalid={fromError !== null}
								placeholder="now"
								className={cn(
									"pr-9",
									fromError !== null && "border-border-destructive",
								)}
								value={fromField.text}
								onChange={(event) => {
									setFromField({ text: event.target.value, touched: true });
								}}
								onBlur={normalizeFrom}
							/>
							<button
								type="button"
								tabIndex={-1}
								aria-label="Pick start date and time"
								className="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer text-content-secondary hover:text-content-primary"
								onClick={() =>
									openPicker(fromPickerRef.current, fromField.text)
								}
							>
								<CalendarIcon className="size-4" />
							</button>
							<input
								ref={fromPickerRef}
								type="datetime-local"
								tabIndex={-1}
								aria-hidden="true"
								className="pointer-events-none absolute h-0 w-0 opacity-0"
								// Chrome and WebKit both fire input per pick; change only
								// fires on dismissal, which is too late for live feedback.
								onInput={(event) => pickFrom(event.currentTarget.value)}
							/>
						</div>
						{fromError !== null && (
							<span className="text-sm text-content-destructive">
								{fromError}
							</span>
						)}
					</div>
					<div className="flex flex-col gap-1.5">
						<div className="flex items-center justify-between">
							<Label htmlFor="time-range-to" className="text-content-primary">
								To
							</Label>
							<button
								type="button"
								tabIndex={-1}
								className="cursor-pointer border-none bg-transparent p-0 text-xs font-normal text-content-secondary hover:text-content-primary"
								onClick={() => setToField({ text: "now", touched: true })}
							>
								[now]
							</button>
						</div>
						<div className="relative">
							<Input
								id="time-range-to"
								aria-label="End of time range"
								aria-invalid={toError !== null}
								placeholder="now"
								className={cn(
									"pr-9",
									toError !== null && "border-border-destructive",
								)}
								value={toField.text}
								onChange={(event) => {
									setToField({ text: event.target.value, touched: true });
								}}
								onBlur={normalizeTo}
							/>
							<button
								type="button"
								tabIndex={-1}
								aria-label="Pick end date and time"
								className="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer text-content-secondary hover:text-content-primary"
								onClick={() => openPicker(toPickerRef.current, toField.text)}
							>
								<CalendarIcon className="size-4" />
							</button>
							<input
								ref={toPickerRef}
								type="datetime-local"
								tabIndex={-1}
								aria-hidden="true"
								className="pointer-events-none absolute h-0 w-0 opacity-0"
								onInput={(event) => pickTo(event.currentTarget.value)}
							/>
						</div>
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
			</PopoverContent>
		</Popover>
	);
};
