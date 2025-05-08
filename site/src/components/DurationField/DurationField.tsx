import FormHelperText from "@mui/material/FormHelperText";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import { ChevronDown as KeyboardArrowDown } from "lucide-react";
import { type FC, useEffect, useReducer } from "react";
import {
	type TimeUnit,
	durationInDays,
	durationInHours,
	suggestedTimeUnit,
} from "utils/time";

type DurationFieldProps = Omit<TextFieldProps, "value" | "onChange"> & {
	valueMs: number;
	onChange: (value: number) => void;
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

			if (
				action.unit === "days" &&
				!canConvertDurationToDays(currentDurationMs)
			) {
				return state;
			}

			return {
				unit: action.unit,
				durationFieldValue:
					action.unit === "hours"
						? durationInHours(currentDurationMs).toString()
						: durationInDays(currentDurationMs).toString(),
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
		...textFieldProps
	} = props;
	const [state, dispatch] = useReducer(reducer, initState(parentValueMs));
	const currentDurationMs = durationInMs(state.durationFieldValue, state.unit);

	useEffect(() => {
		if (parentValueMs !== currentDurationMs) {
			dispatch({ type: "SYNC_WITH_PARENT", parentValueMs });
		}
	}, [currentDurationMs, parentValueMs]);

	return (
		<div>
			<div
				css={{
					display: "flex",
					gap: 8,
				}}
			>
				<TextField
					{...textFieldProps}
					fullWidth
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
					inputProps={{
						step: 1,
					}}
				/>
				<Select
					disabled={props.disabled}
					css={{ width: 120, "& .MuiSelect-icon": { padding: 2 } }}
					value={state.unit}
					onChange={(e) => {
						const unit = e.target.value as TimeUnit;
						dispatch({
							type: "CHANGE_TIME_UNIT",
							unit,
						});
					}}
					inputProps={{ "aria-label": "Time unit" }}
					IconComponent={KeyboardArrowDown}
				>
					<MenuItem value="hours">Hours</MenuItem>
					<MenuItem
						value="days"
						disabled={!canConvertDurationToDays(currentDurationMs)}
					>
						Days
					</MenuItem>
				</Select>
			</div>

			{helperText && (
				<FormHelperText error={props.error}>{helperText}</FormHelperText>
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

function canConvertDurationToDays(duration: number): boolean {
	return Number.isInteger(durationInDays(duration));
}
