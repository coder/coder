import { type FC } from "react";
import { TemplateAutostartRequirementDaysValue } from "utils/schedule";
import Button from "@mui/material/Button";
import { Stack } from "components/Stack/Stack";
import FormHelperText from "@mui/material/FormHelperText";

export interface TemplateScheduleAutostartProps {
  allow_user_autostart?: boolean;
  autostart_requirement_days_of_week: TemplateAutostartRequirementDaysValue[];
  isSubmitting: boolean;
  onChange: (newDaysOfWeek: TemplateAutostartRequirementDaysValue[]) => void;
}

export const TemplateScheduleAutostart: FC<TemplateScheduleAutostartProps> = ({
  autostart_requirement_days_of_week,
  isSubmitting,
  allow_user_autostart,
  onChange,
}) => {
  return (
    <Stack
      direction="column"
      width="100%"
      alignItems="center"
      css={{ marginBottom: "20px" }}
    >
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
            key={day.key}
            css={{ borderRadius: 0 }}
            // TODO: Adding a background color would also help
            color={
              autostart_requirement_days_of_week.includes(day.value)
                ? "primary"
                : "secondary"
            }
            disabled={isSubmitting || !allow_user_autostart}
            onClick={() => {
              if (!autostart_requirement_days_of_week.includes(day.value)) {
                onChange(autostart_requirement_days_of_week.concat(day.value));
              } else {
                onChange(
                  autostart_requirement_days_of_week.filter(
                    (obj) => obj !== day.value,
                  ),
                );
              }
            }}
          >
            {day.key}
          </Button>
        ))}
      </Stack>
      <FormHelperText>
        <AutostartHelperText
          allowed={allow_user_autostart}
          days={autostart_requirement_days_of_week}
        />
      </FormHelperText>
    </Stack>
  );
};

export const sortedDays = [
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
  "sunday",
] as TemplateAutostartRequirementDaysValue[];

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
