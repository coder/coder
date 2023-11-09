import Badge from "@mui/material/Badge";
import { withStyles } from "@mui/styles";
import { useTheme } from "@emotion/react";
import { type FC } from "react";
import type { WorkspaceBuild } from "api/typesGenerated";
import { getDisplayWorkspaceBuildStatus } from "utils/workspace";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";
import type { PaletteIndex } from "theme/mui";
import { BuildIcon } from "components/BuildIcon/BuildIcon";

interface StylesBadgeProps {
  type: PaletteIndex;
}

const StyledBadge = withStyles((theme) => ({
  badge: {
    backgroundColor: ({ type }: StylesBadgeProps) =>
      theme.deprecated.palette[type].light,
    borderRadius: "100%",
    width: 8,
    minWidth: 8,
    height: 8,
    display: "block",
    padding: 0,
  },
}))(Badge);

export interface BuildAvatarProps {
  build: WorkspaceBuild;
  size?: AvatarProps["size"];
}

export const BuildAvatar: FC<BuildAvatarProps> = ({ build, size }) => {
  const theme = useTheme();
  const displayBuildStatus = getDisplayWorkspaceBuildStatus(theme, build);

  return (
    <StyledBadge
      role="status"
      type={displayBuildStatus.type}
      aria-label={displayBuildStatus.status}
      title={displayBuildStatus.status}
      overlap="circular"
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
      badgeContent={<div></div>}
    >
      <Avatar size={size} colorScheme="darken">
        <BuildIcon transition={build.transition} />
      </Avatar>
    </StyledBadge>
  );
};
