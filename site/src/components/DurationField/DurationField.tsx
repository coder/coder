import {
	type ComponentProps,
	type FC,
	type ReactNode,
	useId,
	useState,
} from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { cn } from "#/utils/cn";
import {
	durationInDays,
	durationInHours,
	suggestedTimeUnit,
	type TimeUnit,
} from "#/utils/time";

type DurationFieldProps = Omit<
	ComponentProps<typeof Input>,
	"value" | "onChange" | "type"
> & {
	valueMs: number;
	onChange: (value: number) => void;
	label?: string;
	error?: boolean;
	helperText?: ReactNode;
};

function toMs(value: string, unit: TimeUnit): number {
	const n = Number.parseInt(value, 10);
	if (Number.isNaN(n)) {
		return 0;
	}
	return unit === "hours" ? hoursToDuration(n) : daysToDuration(n);
}

function toDisplayValue(ms: number, unit: TimeUnit): string {
	return unit === "hours"
		? durationInHours(ms).toString()
		: durationInDays(ms).toString();
}

export const DurationField: FC<DurationFieldProps> = ({
	valueMs,
	onChange,
	label,
	error,
	helperText,
	id: idProp,
	className,
	disabled,
	...inputProps
}) => {
	const generatedId = useId();
	const id = idProp ?? generatedId;
	const helperId = `${id}-helper`;
	const [unit, setUnit] = useState<TimeUnit>(() => suggestedTimeUnit(valueMs));
	const [text, setText] = useState(() => toDisplayValue(valueMs, unit));
	const [prevValueMs, setPrevValueMs] = useState(valueMs);

	// Keep local display state in sync when the parent value changes
	// independently (for example, toggling a cleanup switch that resets TTL).
	//
	// We track the previous prop instead of comparing against a value
	// round-tripped through toMs(). toMs() calls Number.parseInt(), which
	// truncates fractional units, so a valid but non-integer TTL (e.g. 90
	// minutes displayed as "1.5" hours) would never match valueMs and would
	// loop this render-phase setState forever, crashing the page.
	if (valueMs !== prevValueMs) {
		setPrevValueMs(valueMs);
		// Only overwrite what the user is editing when the incoming value truly
		// differs from what the field currently represents. This preserves an
		// empty input while the user is clearing it.
		if (valueMs !== toMs(text, unit)) {
			const newUnit = suggestedTimeUnit(valueMs);
			setUnit(newUnit);
			setText(toDisplayValue(valueMs, newUnit));
		}
	}

	const handleTextChange = (raw: string) => {
		const digits = raw.replace(/\D/g, "");
		setText(digits);

		const ms = toMs(digits, unit);
		if (ms !== valueMs) {
			onChange(ms);
		}
	};

	const handleUnitChange = (newUnit: TimeUnit) => {
		const currentMs = toMs(text, unit);

		// When switching to days, round up to the nearest day so a partial day
		// does not silently shrink the configured duration.
		const newMs =
			newUnit === "hours"
				? currentMs
				: daysToDuration(Math.ceil(durationInDays(currentMs)));

		setUnit(newUnit);
		setText(toDisplayValue(newMs, newUnit));

		if (newMs !== valueMs) {
			onChange(newMs);
		}
	};

	return (
		<div className={cn("flex flex-col gap-2", className)}>
			{label && <Label htmlFor={id}>{label}</Label>}
			<div className="flex gap-2">
				<Input
					{...inputProps}
					id={id}
					value={text}
					onChange={(e) => handleTextChange(e.currentTarget.value)}
					disabled={disabled}
					inputMode="numeric"
					pattern="[0-9]*"
					aria-invalid={error}
					aria-describedby={helperText ? helperId : undefined}
					className="w-full min-w-0"
				/>
				<Select
					value={unit}
					onValueChange={(value) => {
						if (value === "hours" || value === "days") {
							handleUnitChange(value);
						}
					}}
					disabled={disabled}
				>
					<SelectTrigger className="w-[120px] flex-none" aria-label="Time unit">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="hours">Hours</SelectItem>
						<SelectItem value="days">Days</SelectItem>
					</SelectContent>
				</Select>
			</div>

			{helperText && (
				<span
					id={helperId}
					className={cn(
						"text-xs",
						error ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{helperText}
				</span>
			)}
		</div>
	);
};

function hoursToDuration(hours: number): number {
	return hours * 60 * 60 * 1000;
}

function daysToDuration(days: number): number {
	return days * 24 * hoursToDuration(1);
}
