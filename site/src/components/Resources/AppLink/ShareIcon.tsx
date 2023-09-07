import PublicOutlinedIcon from "@mui/icons-material/PublicOutlined";
import GroupOutlinedIcon from "@mui/icons-material/GroupOutlined";
import LaunchOutlinedIcon from "@mui/icons-material/LaunchOutlined";
import * as TypesGen from "../../../api/typesGenerated";
import Tooltip from "@mui/material/Tooltip";
import { useTranslation } from "react-i18next";

export interface ShareIconProps {
  app: TypesGen.WorkspaceApp;
}

export const ShareIcon = ({ app }: ShareIconProps) => {
  const { t } = useTranslation("agent");
  if (app.external) {
    return (
      <Tooltip title={t("shareTooltip.external")}>
        <LaunchOutlinedIcon />
      </Tooltip>
    );
  }
  if (app.sharing_level === "authenticated") {
    return (
      <Tooltip title={t("shareTooltip.authenticated")}>
        <GroupOutlinedIcon />
      </Tooltip>
    );
  }
  if (app.sharing_level === "public") {
    return (
      <Tooltip title={t("shareTooltip.public")}>
        <PublicOutlinedIcon />
      </Tooltip>
    );
  }

  return null;
};
