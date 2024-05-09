import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import FormHelperText from "@mui/material/FormHelperText";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import TextField from "@mui/material/TextField";
import { type ReactNode, useState, type FC } from "react";

type TimeUnit = "days" | "hours";

// Value should be in milliseconds or undefined. Undefined means no value.
type DurationValue = number | undefined;

type DurationFieldProps = {
  label: string;
  value: DurationValue;
  disabled?: boolean;
  helperText?: ReactNode;
  onChange: (value: DurationValue) => void;
};

export const DurationField: FC<DurationFieldProps> = (props) => {
  const { label, value, disabled, helperText, onChange } = props;
  const [timeUnit, setTimeUnit] = useState<TimeUnit>(() => {
    if (!value) {
      return "hours";
    }

    return Number.isInteger(durationToDays(value)) ? "days" : "hours";
  });

  return (
    <div>
      <div
        css={{
          display: "flex",
          gap: 8,
        }}
      >
        <TextField
          type="number"
          css={{ maxWidth: 160 }}
          label={label}
          disabled={disabled}
          value={
            !value
              ? ""
              : timeUnit === "hours"
                ? durationToHours(value)
                : durationToDays(value)
          }
          onChange={(e) => {
            if (e.target.value === "") {
              onChange(undefined);
            }

            let value = parseInt(e.target.value);

            if (Number.isNaN(value)) {
              return;
            }

            // Avoid negative values
            value = Math.abs(value);

            onChange(
              timeUnit === "hours"
                ? hoursToDuration(value)
                : daysToDuration(value),
            );
          }}
          inputProps={{
            step: 1,
          }}
        />
        <Select
          disabled={disabled}
          css={{ width: 120, "& .MuiSelect-icon": { padding: 2 } }}
          value={timeUnit}
          onChange={(e) => {
            setTimeUnit(e.target.value as TimeUnit);
          }}
          inputProps={{ "aria-label": "Time unit" }}
          IconComponent={KeyboardArrowDown}
        >
          <MenuItem
            value="hours"
            disabled={Boolean(value && !canConvertDurationToHours(value))}
          >
            Hours
          </MenuItem>
          <MenuItem
            value="days"
            disabled={Boolean(value && !canConvertDurationToDays(value))}
          >
            Days
          </MenuItem>
        </Select>
      </div>

      {helperText && <FormHelperText>{helperText}</FormHelperText>}
    </div>
  );
};

function durationToHours(duration: number): number {
  return duration / 1000 / 60 / 60;
}

function hoursToDuration(hours: number): number {
  return hours * 60 * 60 * 1000;
}

function durationToDays(duration: number): number {
  return duration / 1000 / 60 / 60 / 24;
}

function daysToDuration(days: number): number {
  return days * 24 * 60 * 60 * 1000;
}

function canConvertDurationToDays(duration: number): boolean {
  return Number.isInteger(durationToDays(duration));
}

function canConvertDurationToHours(duration: number): boolean {
  return Number.isInteger(durationToHours(duration));
}
