import { Avatar } from "components/Avatar/Avatar"
import Badge from "@material-ui/core/Badge"
import { withStyles } from "@material-ui/core/styles"
import Group from "@material-ui/icons/Group"
import { FC } from "react"

const StyledBadge = withStyles((theme) => ({
  badge: {
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: "100%",
    width: 24,
    height: 24,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",

    "& svg": {
      width: 14,
      height: 14,
    },
  },
}))(Badge)

export type GroupAvatarProps = {
  name: string
  avatarURL?: string
}

export const GroupAvatar: FC<GroupAvatarProps> = ({ name, avatarURL }) => {
  return (
    <StyledBadge
      overlap="circular"
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
      badgeContent={<Group />}
    >
      <Avatar src={avatarURL}>{name}</Avatar>
    </StyledBadge>
  )
}
