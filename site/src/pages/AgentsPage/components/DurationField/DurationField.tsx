import dayjs from "dayjs";
import { type FC, type ReactNode, useState } from "react";
import { Input } from "#/components/Input/Input";
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

type DurationFieldProps = {
	valueMs: number;
	onChange: (value: number) => void;
	label?: string;
	disabled?: boolean;
	error?: boolean;
	helperText?: ReactNode;
	className?: string;
};

function toMs(value: string, unit: TimeUnit): number {
	const n = Number.parseInt(value, 10);
	if (Number.isNaN(n)) {
		return 0;
	}
	return unit === "hours"
		? dayjs.duration(n, "hours").asMilliseconds()
		: dayjs.duration(n, "days").asMilliseconds();
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
	disabled,
	error,
	helperText,
	className,
}) => {
	const [unit, setUnit] = useState<TimeUnit>(() => suggestedTimeUnit(valueMs));
	const [text, setText] = useState(() => toDisplayValue(valueMs, unit));

	// Adjust local state when the parent value diverges from ours.
	const localMs = toMs(text, unit);
	if (valueMs !== localMs) {
		const newUnit = suggestedTimeUnit(valueMs);
		setUnit(newUnit);
		setText(toDisplayValue(valueMs, newUnit));
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

		let newMs: number;
		if (newUnit === "hours") {
			newMs = currentMs;
		} else {
			const days = Math.ceil(durationInDays(currentMs));
			newMs = dayjs.duration(days, "days").asMilliseconds();
		}

		setUnit(newUnit);
		setText(toDisplayValue(newMs, newUnit));

		if (newMs !== valueMs) {
			onChange(newMs);
		}
	};

	return (
		<div className={cn("space-y-1", className)}>
			<div className="flex gap-2">
				<Input
					value={text}
					onChange={(e) => handleTextChange(e.currentTarget.value)}
					aria-label={label}
					aria-invalid={error}
					disabled={disabled}
					className="flex-1"
				/>
				<Select
					value={unit}
					onValueChange={(v: string) => handleUnitChange(v as TimeUnit)}
					disabled={disabled}
				>
					<SelectTrigger className="w-[120px]" aria-label="Time unit">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="hours">Hours</SelectItem>
						<SelectItem value="days">Days</SelectItem>
					</SelectContent>
				</Select>
			</div>

			{helperText && (
				<p
					className={cn(
						"m-0 text-xs",
						error ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{helperText}
				</p>
			)}
		</div>
	);
};
