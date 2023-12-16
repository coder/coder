import { type FC } from "react";
import type { Template } from "api/typesGenerated";
import type { TemplateAutostopRequirementDaysValue } from "utils/schedule";

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

interface AutostopRequirementDaysHelperTextProps {
  days: TemplateAutostopRequirementDaysValue;
}

export const AutostopRequirementDaysHelperText: FC<
  AutostopRequirementDaysHelperTextProps
> = ({ days = "off" }) => {
  return <span>{autostopRequirementDescriptions[days]}</span>;
};

interface AutostopRequirementWeeksHelperTextProps {
  days: TemplateAutostopRequirementDaysValue;
  weeks: number;
}

export const AutostopRequirementWeeksHelperText: FC<
  AutostopRequirementWeeksHelperTextProps
> = ({ days, weeks }) => {
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
