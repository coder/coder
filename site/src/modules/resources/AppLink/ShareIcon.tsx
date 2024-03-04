import GroupOutlinedIcon from "@mui/icons-material/GroupOutlined";
import LaunchOutlinedIcon from "@mui/icons-material/LaunchOutlined";
import PublicOutlinedIcon from "@mui/icons-material/PublicOutlined";
import Tooltip from "@mui/material/Tooltip";
import type * as TypesGen from "api/typesGenerated";

export interface ShareIconProps {
  app: TypesGen.WorkspaceApp;
}

export const ShareIcon = ({ app }: ShareIconProps) => {
  if (app.external) {
    return (
      <Tooltip title="Open external URL">
        <LaunchOutlinedIcon />
      </Tooltip>
    );
  }
  if (app.sharing_level === "authenticated") {
    return (
      <Tooltip title="Shared with all authenticated users">
        <GroupOutlinedIcon />
      </Tooltip>
    );
  }
  if (app.sharing_level === "public") {
    return (
      <Tooltip title="Shared publicly">
        <PublicOutlinedIcon />
      </Tooltip>
    );
  }

  return null;
};
