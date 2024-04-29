import Button from "@mui/material/Button";
import FormHelperText from "@mui/material/FormHelperText";
import type { FC } from "react";
import { Stack } from "components/Stack/Stack";
import {
  sortedDays,
  type TemplateAutostartRequirementDaysValue,
} from "utils/schedule";

export interface TemplateScheduleAutostartProps {
  enabled: boolean;
  value: TemplateAutostartRequirementDaysValue[];
  isSubmitting: boolean;
  onChange: (value: TemplateAutostartRequirementDaysValue[]) => void;
}

export const TemplateScheduleAutostart: FC<TemplateScheduleAutostartProps> = ({
  value,
  isSubmitting,
  enabled,
  onChange,
}) => {
  return (
    <Stack width="100%" alignItems="start" spacing={1}>
      <Stack
        direction="row"
        spacing={0}
        alignItems="baseline"
        justifyContent="center"
        css={{ width: "100%" }}
      >
        {(
          [
            { value: "monday", key: "Mon" },
            { value: "tuesday", key: "Tue" },
            { value: "wednesday", key: "Wed" },
            { value: "thursday", key: "Thu" },
            { value: "friday", key: "Fri" },
            { value: "saturday", key: "Sat" },
            { value: "sunday", key: "Sun" },
          ] as {
            value: TemplateAutostartRequirementDaysValue;
            key: string;
          }[]
        ).map((day) => (
          <Button
            fullWidth
            key={day.key}
            css={{ borderRadius: 0 }}
            // TODO: Adding a background color would also help
            color={value.includes(day.value) ? "primary" : "secondary"}
            disabled={isSubmitting || !enabled}
            onClick={() => {
              if (!value.includes(day.value)) {
                onChange(value.concat(day.value));
              } else {
                onChange(value.filter((obj) => obj !== day.value));
              }
            }}
          >
            {day.key}
          </Button>
        ))}
      </Stack>
      <FormHelperText>
        <AutostartHelperText allowed={enabled} days={value} />
      </FormHelperText>
    </Stack>
  );
};

interface AutostartHelperTextProps {
  allowed?: boolean;
  days: TemplateAutostartRequirementDaysValue[];
}

const AutostartHelperText: FC<AutostartHelperTextProps> = ({
  allowed,
  days: unsortedDays,
}) => {
  if (!allowed) {
    return <span>Workspaces are not allowed to auto start.</span>;
  }

  const days = new Set(unsortedDays);

  if (days.size === 7) {
    // If every day is allowed, no more explaining is needed.
    return <span>Workspaces are allowed to auto start on any day.</span>;
  }
  if (days.size === 0) {
    return (
      <span>
        Workspaces will never auto start. This is effectively the same as
        disabling autostart.
      </span>
    );
  }

  let daymsg = "Workspaces will never auto start on the weekends.";
  if (days.size !== 5 || days.has("saturday") || days.has("sunday")) {
    daymsg = `Workspaces can autostart on ${sortedDays
      .filter((day) => days.has(day))
      .join(", ")}.`;
  }

  return (
    <span>{daymsg} These days are relative to the user&apos;s timezone.</span>
  );
};
