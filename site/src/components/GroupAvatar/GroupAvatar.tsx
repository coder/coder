import Badge from "@mui/material/Badge";
import Group from "@mui/icons-material/Group";
import { useTheme } from "@emotion/react";
import { type FC } from "react";
import { Avatar } from "components/Avatar/Avatar";

export interface GroupAvatarProps {
  name: string;
  avatarURL?: string;
}

export const GroupAvatar: FC<GroupAvatarProps> = ({ name, avatarURL }) => {
  const theme = useTheme();

  return (
    <Badge
      overlap="circular"
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
      badgeContent={<Group />}
      css={{
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
      }}
    >
      <Avatar src={avatarURL} background>
        {name}
      </Avatar>
    </Badge>
  );
};
