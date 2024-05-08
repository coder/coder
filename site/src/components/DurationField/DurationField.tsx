import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import TextField from "@mui/material/TextField";
import { useState, type FC } from "react";

type TimeUnit = "days" | "hours";

// Value should be in milliseconds or undefined. Undefined means no value.
type DurationValue = number | undefined;

type DurationFieldProps = {
  label: string;
  value: DurationValue;
  onChange: (value: DurationValue) => void;
};

export const DurationField: FC<DurationFieldProps> = (props) => {
  const { label, value, onChange } = props;
  const [timeUnit, setTimeUnit] = useState<TimeUnit>(() => {
    if (!value) {
      return "hours";
    }

    return Number.isInteger(durationToDays(value)) ? "days" : "hours";
  });

  return (
    <div
      css={{
        display: "flex",
        gap: 8,
      }}
    >
      <TextField
        css={{ maxWidth: 160 }}
        label={label}
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

          const value = parseInt(e.target.value);

          if (Number.isNaN(value)) {
            return;
          }

          onChange(
            timeUnit === "hours"
              ? hoursToDuration(value)
              : daysToDuration(value),
          );
        }}
        inputProps={{
          step: 1,
          type: "number",
        }}
      />
      <Select
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
