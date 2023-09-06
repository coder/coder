import Badge from "@mui/material/Badge"
import { useTheme, withStyles } from "@mui/styles"
import { FC } from "react"
import { WorkspaceBuild } from "api/typesGenerated"
import { getDisplayWorkspaceBuildStatus } from "utils/workspace"
import { Avatar, AvatarProps } from "components/Avatar/Avatar"
import { PaletteIndex } from "theme/theme"
import { Theme } from "@mui/material/styles"
import { BuildIcon } from "components/BuildIcon/BuildIcon"

interface StylesBadgeProps {
  type: PaletteIndex
}

const StyledBadge = withStyles((theme) => ({
  badge: {
    backgroundColor: ({ type }: StylesBadgeProps) => theme.palette[type].light,
    borderRadius: "100%",
    width: 8,
    minWidth: 8,
    height: 8,
    display: "block",
    padding: 0,
  },
}))(Badge)

export interface BuildAvatarProps {
  build: WorkspaceBuild
  size?: AvatarProps["size"]
}

export const BuildAvatar: FC<BuildAvatarProps> = ({ build, size }) => {
  const theme = useTheme<Theme>()
  const displayBuildStatus = getDisplayWorkspaceBuildStatus(theme, build)

  return (
    <StyledBadge
      role="status"
      type={displayBuildStatus.type}
      arial-label={displayBuildStatus.status}
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
  )
}
