import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import FormHelperText from "@mui/material/FormHelperText";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import { useState, type FC, useEffect } from "react";
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

export const DurationField: FC<DurationFieldProps> = (props) => {
  const {
    valueMs: parentValueMs,
    onChange,
    helperText,
    ...textFieldProps
  } = props;
  const [state, setState] = useState<State>(() => initState(parentValueMs));
  const currentDurationMs = durationInMs(state.durationFieldValue, state.unit);

  useEffect(() => {
    if (parentValueMs !== currentDurationMs) {
      setState(initState(parentValueMs));
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
          type="number"
          css={{ maxWidth: 160 }}
          value={state.durationFieldValue}
          onChange={(e) => {
            const durationFieldValue = e.currentTarget.value;

            setState((state) => ({
              ...state,
              durationFieldValue,
            }));

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
            setState(() => ({
              unit,
              durationFieldValue:
                unit === "hours"
                  ? durationInHours(currentDurationMs).toString()
                  : durationInDays(currentDurationMs).toString(),
            }));
          }}
          inputProps={{ "aria-label": "Time unit" }}
          IconComponent={KeyboardArrowDown}
        >
          <MenuItem
            value="hours"
            disabled={!canConvertDurationToHours(currentDurationMs)}
          >
            Hours
          </MenuItem>
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

function durationInMs(durationFieldValue: string, unit: TimeUnit): number {
  const durationInMs = parseInt(durationFieldValue);

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

function canConvertDurationToHours(duration: number): boolean {
  return Number.isInteger(durationInHours(duration));
}
