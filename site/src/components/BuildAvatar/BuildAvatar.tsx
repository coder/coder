import Badge from "@mui/material/Badge";
import { useTheme } from "@emotion/react";
import { type FC } from "react";
import type { WorkspaceBuild } from "api/typesGenerated";
import { getDisplayWorkspaceBuildStatus } from "utils/workspace";
import { Avatar, AvatarProps } from "components/Avatar/Avatar";
import { BuildIcon } from "components/BuildIcon/BuildIcon";

export interface BuildAvatarProps {
  build: WorkspaceBuild;
  size?: AvatarProps["size"];
}

export const BuildAvatar: FC<BuildAvatarProps> = ({ build, size }) => {
  const theme = useTheme();
  const displayBuildStatus = getDisplayWorkspaceBuildStatus(theme, build);

  return (
    <Badge
      role="status"
      aria-label={displayBuildStatus.status}
      title={displayBuildStatus.status}
      overlap="circular"
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
      badgeContent={<div></div>}
      css={{
        backgroundColor: theme.palette[displayBuildStatus.type].light,
        borderRadius: "100%",
        width: 8,
        minWidth: 8,
        height: 8,
        display: "block",
        padding: 0,
      }}
    >
      <Avatar background size={size}>
        <BuildIcon transition={build.transition} />
      </Avatar>
    </Badge>
  );
};
