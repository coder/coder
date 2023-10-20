import { FC } from "react";
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

export const TemplateScheduleAutostart: FC<
  React.PropsWithChildren<TemplateScheduleAutostartProps>
> = ({
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
      css={{
        marginBottom: "20px",
      }}
    >
      <Stack
        direction="row"
        css={{
          width: "100%",
        }}
        spacing={0}
        alignItems="baseline"
        justifyContent="center"
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
            css={{
              borderRadius: "0px",
            }}
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
        <AutostartRequirementDaysHelperText
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

const AutostartRequirementDaysHelperText: FC<{
  allowed?: boolean;
  days: TemplateAutostartRequirementDaysValue[];
}> = ({ allowed, days: unsortedDays }) => {
  if (!allowed) {
    return <span>Workspaces are not allowed to auto start.</span>;
  }
  // Sort the days
  const days = unsortedDays.sort(
    (a, b) => sortedDays.indexOf(a) - sortedDays.indexOf(b),
  );

  let daymsg = `Workspaces can autostart on ${days.join(", ")}.`;
  if (days.length === 7) {
    // If every day is allowed, no more explaining is needed.
    return <span>Workspaces are allowed to auto start on any day.</span>;
  }
  if (days.length === 0) {
    return (
      <span>
        Workspaces will never auto start. This is effectively the same as
        disabling autostart.
      </span>
    );
  }
  if (
    days.length === 5 &&
    !days.includes("saturday") &&
    !days.includes("sunday")
  ) {
    daymsg = "Workspaces will never auto start on the weekends.";
  }
  return (
    <span>{daymsg} These days are relative to the user&apos;s timezone.</span>
  );
};
