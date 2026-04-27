import {
	type FC,
	type InputHTMLAttributes,
	type ReactNode,
	useEffect,
	useId,
	useReducer,
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
	InputHTMLAttributes<HTMLInputElement>,
	"onChange" | "value"
> & {
	valueMs: number;
	onChange: (value: number) => void;
	label?: ReactNode;
	helperText?: ReactNode;
	error?: boolean;
	fullWidth?: boolean;
	value?: InputHTMLAttributes<HTMLInputElement>["value"];
};

type State = {
	unit: TimeUnit;
	// Handling empty values as strings in the input simplifies the process,
	// especially when a user clears the input field.
	durationFieldValue: string;
};

type Action =
	| { type: "SYNC_WITH_PARENT"; parentValueMs: number }
	| { type: "CHANGE_DURATION_FIELD_VALUE"; fieldValue: string }
	| { type: "CHANGE_TIME_UNIT"; unit: TimeUnit };

const reducer = (state: State, action: Action): State => {
	switch (action.type) {
		case "SYNC_WITH_PARENT": {
			return initState(action.parentValueMs);
		}
		case "CHANGE_DURATION_FIELD_VALUE": {
			return {
				...state,
				durationFieldValue: action.fieldValue,
			};
		}
		case "CHANGE_TIME_UNIT": {
			const currentDurationMs = durationInMs(
				state.durationFieldValue,
				state.unit,
			);

			return {
				unit: action.unit,
				durationFieldValue:
					action.unit === "hours"
						? durationInHours(currentDurationMs).toString()
						: Math.ceil(durationInDays(currentDurationMs)).toString(),
			};
		}
		default: {
			return state;
		}
	}
};

export const DurationField: FC<DurationFieldProps> = (props) => {
	const {
		valueMs: parentValueMs,
		onChange,
		helperText,
		error,
		label,
		fullWidth: _fullWidth,
		value: _value,
		id,
		className,
		disabled,
		inputMode,
		step,
		"aria-describedby": ariaDescribedBy,
		...inputProps
	} = props;
	const generatedId = useId();
	const inputId = id ?? generatedId;
	const helperTextId = helperText ? `${inputId}-helper-text` : undefined;
	const describedBy = [ariaDescribedBy, helperTextId].filter(Boolean).join(" ");
	const [state, dispatch] = useReducer(reducer, initState(parentValueMs));
	const currentDurationMs = durationInMs(state.durationFieldValue, state.unit);

	useEffect(() => {
		if (parentValueMs !== currentDurationMs) {
			dispatch({ type: "SYNC_WITH_PARENT", parentValueMs });
		}
	}, [currentDurationMs, parentValueMs]);

	const handleUnitChange = (value: string) => {
		if (!isTimeUnit(value)) {
			return;
		}

		const unit = value;
		dispatch({
			type: "CHANGE_TIME_UNIT",
			unit,
		});

		// Calculate the new duration in ms after changing the unit.
		// Important: When changing from hours to days, we need to round up to
		// nearest day but keep the millisecond value consistent for the parent
		// component.
		let newDurationMs: number;
		if (unit === "hours") {
			// When switching to hours, use the current milliseconds to get exact
			// hours.
			newDurationMs = currentDurationMs;
		} else {
			// When switching to days, round up to the nearest day.
			const daysValue = Math.ceil(durationInDays(currentDurationMs));
			newDurationMs = daysToDuration(daysValue);
		}

		// Notify parent component if the value has changed.
		if (newDurationMs !== parentValueMs) {
			onChange(newDurationMs);
		}
	};

	return (
		<div>
			{label && (
				<Label htmlFor={inputId} className="mb-2 block">
					{label}
				</Label>
			)}

			<div className="flex gap-2">
				<Input
					{...inputProps}
					id={inputId}
					className={cn("min-w-0 flex-1", className)}
					disabled={disabled}
					value={state.durationFieldValue}
					onChange={(e) => {
						const durationFieldValue = intMask(e.currentTarget.value);

						dispatch({
							type: "CHANGE_DURATION_FIELD_VALUE",
							fieldValue: durationFieldValue,
						});

						const newDurationInMs = durationInMs(
							durationFieldValue,
							state.unit,
						);
						if (newDurationInMs !== parentValueMs) {
							onChange(newDurationInMs);
						}
					}}
					step={step ?? 1}
					inputMode={inputMode ?? "numeric"}
					aria-invalid={error || undefined}
					aria-describedby={describedBy || undefined}
				/>
				<Select
					disabled={disabled}
					value={state.unit}
					onValueChange={handleUnitChange}
				>
					<SelectTrigger
						disabled={disabled}
						aria-label="Time unit"
						className="w-[120px] shrink-0"
					>
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
					id={helperTextId}
					className={cn(
						"mt-2 text-xs",
						error ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{helperText}
				</p>
			)}
		</div>
	);
};

function initState(value: number): State {
	const unit = suggestedTimeUnit(value);
	const durationFieldValue =
		unit === "hours"
			? durationInHours(value).toString()
			: durationInDays(value).toString();

	return {
		unit,
		durationFieldValue,
	};
}

function intMask(value: string): string {
	return value.replace(/\D/g, "");
}

function durationInMs(durationFieldValue: string, unit: TimeUnit): number {
	const durationInMs = Number.parseInt(durationFieldValue, 10);

	if (Number.isNaN(durationInMs)) {
		return 0;
	}

	return unit === "hours"
		? hoursToDuration(durationInMs)
		: daysToDuration(durationInMs);
}

function hoursToDuration(hours: number): number {
	return hours * 60 * 60 * 1000;
}

function daysToDuration(days: number): number {
	return days * 24 * hoursToDuration(1);
}

function isTimeUnit(value: string): value is TimeUnit {
	return value === "hours" || value === "days";
}
