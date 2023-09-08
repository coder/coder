import { Template } from "api/typesGenerated";

export type TemplateAutostopRequirementDaysValue =
  | "off"
  | "daily"
  | "saturday"
  | "sunday";

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

export const calculateAutostopRequirementDaysValue = (
  value: TemplateAutostopRequirementDaysValue,
): Template["autostop_requirement"]["days_of_week"] => {
  switch (value) {
    case "daily":
      return [
        "monday",
        "tuesday",
        "wednesday",
        "thursday",
        "friday",
        "saturday",
        "sunday",
      ];
    case "saturday":
      return ["saturday"];
    case "sunday":
      return ["sunday"];
  }

  return [];
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
