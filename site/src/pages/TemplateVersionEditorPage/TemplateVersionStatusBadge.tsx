import { type TemplateVersion } from "api/typesGenerated";
import { type FC, type ReactNode } from "react";
import ErrorIcon from "@mui/icons-material/ErrorOutline";
import CheckIcon from "@mui/icons-material/CheckOutlined";
import { Pill, PillSpinner } from "components/Pill/Pill";
import { ThemeRole } from "theme/experimental";

interface TemplateVersionStatusBadgeProps {
  version: TemplateVersion;
}

export const TemplateVersionStatusBadge: FC<
  TemplateVersionStatusBadgeProps
> = ({ version }) => {
  const { text, icon, color } = getStatus(version);
  return (
    <Pill icon={icon} color={color} title={`Build status is ${text}`}>
      {text}
    </Pill>
  );
};

export const getStatus = (
  version: TemplateVersion,
): {
  color?: ThemeRole;
  text: string;
  icon: ReactNode;
} => {
  switch (version.job.status) {
    case "running":
      return {
        color: "info",
        text: "Running",
        icon: <PillSpinner />,
      };
    case "pending":
      return {
        color: "info",
        text: "Pending",
        icon: <PillSpinner />,
      };
    case "canceling":
      return {
        color: "warning",
        text: "Canceling",
        icon: <PillSpinner />,
      };
    case "canceled":
      return {
        color: "warning",
        text: "Canceled",
        icon: <ErrorIcon />,
      };
    case "unknown":
    case "failed":
      return {
        color: "error",
        text: "Failed",
        icon: <ErrorIcon />,
      };
    case "succeeded":
      return {
        color: "success",
        text: "Success",
        icon: <CheckIcon />,
      };
  }
};
