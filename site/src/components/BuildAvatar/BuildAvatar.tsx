import Badge from "@mui/material/Badge";
import { css, cx } from "@emotion/css";
import { useTheme } from "@emotion/react";
import { type FC } from "react";
import type { WorkspaceBuild } from "api/typesGenerated";
import { getDisplayWorkspaceBuildStatus } from "utils/workspace";
import { useClassName } from "hooks/useClassName";
import { Avatar, AvatarProps } from "components/Avatar/Avatar";
import { BuildIcon } from "components/BuildIcon/BuildIcon";

export interface BuildAvatarProps {
  build: WorkspaceBuild;
  size?: AvatarProps["size"];
}

export const BuildAvatar: FC<BuildAvatarProps> = ({ build, size }) => {
  const theme = useTheme();
  const { status, type } = getDisplayWorkspaceBuildStatus(theme, build);
  const badgeType = useClassName(
    (css, theme) => css`
      background-color: ${theme.palette[type].light};
    `,
    [type],
  );

  return (
    <Badge
      role="status"
      aria-label={status}
      title={status}
      overlap="circular"
      anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
      badgeContent={<div></div>}
      classes={{ badge: cx(classNames.badge, badgeType) }}
    >
      <Avatar background size={size}>
        <BuildIcon transition={build.transition} />
      </Avatar>
    </Badge>
  );
};

const classNames = {
  badge: css({
    borderRadius: "100%",
    width: 8,
    minWidth: 8,
    height: 8,
    display: "block",
    padding: 0,
  }),
};
