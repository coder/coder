import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { type FC, type ReactNode, useEffect, useReducer } from "react";
import {
	durationInDays,
	durationInHours,
	suggestedTimeUnit,
	type TimeUnit,
} from "utils/time";

interface DurationFieldProps {
	valueMs: number;
	onChange: (value: number) => void;
	label?: string;
	disabled?: boolean;
	helperText?: ReactNode;
	error?: boolean;
	name?: string;
	id?: string;
}

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

export const DurationField: FC<DurationFieldProps> = ({
	valueMs: parentValueMs,
	onChange,
	helperText,
	error,
	label,
	disabled,
	name,
	id,
}) => {
	const [state, dispatch] = useReducer(reducer, initState(parentValueMs));
	const currentDurationMs = durationInMs(state.durationFieldValue, state.unit);

	useEffect(() => {
		if (parentValueMs !== currentDurationMs) {
			dispatch({ type: "SYNC_WITH_PARENT", parentValueMs });
		}
	}, [currentDurationMs, parentValueMs]);

	const inputId = id ?? name;

	return (
		<div className="flex flex-col gap-2">
			{label && <Label htmlFor={inputId}>{label}</Label>}
			<div className="flex gap-2">
				<Input
					id={inputId}
					name={name}
					disabled={disabled}
					className="flex-1"
					value={state.durationFieldValue}
					aria-invalid={error}
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
				/>
				<Select
					disabled={disabled}
					value={state.unit}
					onValueChange={(value) => {
						const unit = value as TimeUnit;
						dispatch({
							type: "CHANGE_TIME_UNIT",
							unit,
						});

						// Calculate the new duration in ms after changing the unit.
						// When changing from hours to days, we round up to the
						// nearest day but keep the ms value consistent for the
						// parent component.
						let newDurationMs: number;
						if (unit === "hours") {
							newDurationMs = currentDurationMs;
						} else {
							const daysValue = Math.ceil(durationInDays(currentDurationMs));
							newDurationMs = daysToDuration(daysValue);
						}

						if (newDurationMs !== parentValueMs) {
							onChange(newDurationMs);
						}
					}}
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
				<span
					className={`text-xs ${error ? "text-content-destructive" : "text-content-secondary"}`}
				>
					{helperText}
				</span>
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
