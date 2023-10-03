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
    <Pill
      icon={icon}
      text={text}
      type={type}
      title={`Build status is ${text}`}
    />
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
        text: "Pending",
        icon: <LoadingIcon />,
        type: "info",
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
