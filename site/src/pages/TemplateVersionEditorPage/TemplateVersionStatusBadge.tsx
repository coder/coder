import { type TemplateVersion } from "api/typesGenerated";
import { type FC, type ReactNode } from "react";
import CircularProgress from "@mui/material/CircularProgress";
import ErrorIcon from "@mui/icons-material/ErrorOutline";
import CheckIcon from "@mui/icons-material/CheckOutlined";
import { Pill, type PillType } from "components/Pill/Pill";

export const TemplateVersionStatusBadge: FC<{
  version: TemplateVersion;
}> = ({ version }) => {
  const { text, icon, type } = getStatus(version);
  return (
    <Pill icon={icon} type={type} title={`Build status is ${text}`}>
      {text}
    </Pill>
  );
};

const LoadingIcon: FC = () => {
  return <CircularProgress size={10} style={{ color: "#FFF" }} />;
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
        icon: <LoadingIcon />,
      };
    case "pending":
      return {
        type: "info",
        text: "Pending",
        icon: <LoadingIcon />,
      };
    case "canceling":
      return {
        type: "warning",
        text: "Canceling",
        icon: <LoadingIcon />,
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
