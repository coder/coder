import { Template } from "api/typesGenerated";
import { useTranslation } from "react-i18next";

export type TemplateAutostopRequirementDaysValue =
  | "off"
  | "daily"
  | "saturday"
  | "sunday";

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
  days,
}: {
  days: TemplateAutostopRequirementDaysValue;
}) => {
  const { t } = useTranslation("templateSettingsPage");

  let str = "off";
  if (days) {
    str = days;
  }

  return <span>{t("autostopRequirementDaysHelperText_" + str)}</span>;
};

export const AutostopRequirementWeeksHelperText = ({
  days,
  weeks,
}: {
  days: TemplateAutostopRequirementDaysValue;
  weeks: number;
}) => {
  const { t } = useTranslation("templateSettingsPage");

  let str = "disabled";
  if (days === "saturday" || days === "sunday") {
    if (weeks === 0 || weeks === 1) {
      str = "one";
    } else {
      str = "other";
    }
  }

  return (
    <span>
      {t("autostopRequirementWeeksHelperText_" + str, { count: weeks })}
    </span>
  );
};
