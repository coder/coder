import { Template } from "api/typesGenerated";
import { FC } from "react";
import {
  TemplateAutostartRequirementDaysValue,
  TemplateAutostopRequirementDaysValue,
} from "utils/schedule";

const autostopRequirementDescriptions = {
  off: "Workspaces are not required to stop periodically.",
  daily:
    "Workspaces are required to be automatically stopped daily in the user's quiet hours and timezone.",
  saturday:
    "Workspaces are required to be automatically stopped every Saturday in the user's quiet hours and timezone.",
  sunday:
    "Workspaces are required to be automatically stopped every Sunday in the user's quiet hours and timezone.",
};

export const convertAutostopRequirementDaysValue = (
  days: Template["autostop_requirement"]["days_of_week"],
): TemplateAutostopRequirementDaysValue => {
  if (days.length === 7) {
    return "daily";
  } else if (days.length === 1 && days[0] === "saturday") {
    return "saturday";
  } else if (days.length === 1 && days[0] === "sunday") {
    return "sunday";
  }

  // On unsupported values we default to "off".
  return "off";
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

export const AutostartRequirementDaysHelperText: FC<{
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

export const AutostopRequirementDaysHelperText = ({
  days = "off",
}: {
  days: TemplateAutostopRequirementDaysValue;
}) => {
  return <span>{autostopRequirementDescriptions[days]}</span>;
};

export const AutostopRequirementWeeksHelperText = ({
  days,
  weeks,
}: {
  days: TemplateAutostopRequirementDaysValue;
  weeks: number;
}) => {
  // Disabled
  if (days !== "saturday" && days !== "sunday") {
    return (
      <span>
        Weeks between required stops cannot be set unless days between required
        stops is Saturday or Sunday.
      </span>
    );
  }

  if (weeks <= 1) {
    return (
      <span>
        Workspaces are required to be automatically stopped every week on the
        specified day in the user&apos;s quiet hours and timezone.
      </span>
    );
  }

  return (
    <span>
      Workspaces are required to be automatically stopped every {weeks} weeks on
      the specified day in the user&apos;s quiet hours and timezone.
    </span>
  );
};
