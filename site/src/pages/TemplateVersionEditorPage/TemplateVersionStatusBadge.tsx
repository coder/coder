import { type TemplateVersion } from "api/typesGenerated";
import { type FC, type ReactNode } from "react";
import ErrorIcon from "@mui/icons-material/ErrorOutline";
import CheckIcon from "@mui/icons-material/CheckOutlined";
import { Pill, PillSpinner, type PillType } from "components/Pill/Pill";

interface TemplateVersionStatusBadgeProps {
  version: TemplateVersion;
}

export const TemplateVersionStatusBadge: FC<
  TemplateVersionStatusBadgeProps
> = ({ version }) => {
  const { text, icon, type } = getStatus(version);
  return (
    <Pill icon={icon} type={type} title={`Build status is ${text}`}>
      {text}
    </Pill>
  );
};

export const getStatus = (
  version: TemplateVersion,
): {
  type?: PillType;
  text: string;
  icon: ReactNode;
} => {
  switch (version.job.status) {
    case "running":
      return {
        type: "info",
        text: "Running",
        icon: <PillSpinner />,
      };
    case "pending":
      return {
        type: "info",
        text: "Pending",
        icon: <PillSpinner />,
      };
    case "canceling":
      return {
        type: "warning",
        text: "Canceling",
        icon: <PillSpinner />,
      };
    case "canceled":
      return {
        type: "warning",
        text: "Canceled",
        icon: <ErrorIcon />,
      };
    case "unknown":
    case "failed":
      return {
        type: "error",
        text: "Failed",
        icon: <ErrorIcon />,
      };
    case "succeeded":
      return {
        type: "success",
        text: "Success",
        icon: <CheckIcon />,
      };
  }
};
