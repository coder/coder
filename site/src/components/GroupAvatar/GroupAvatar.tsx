import Badge from "@mui/material/Badge";
import Group from "@mui/icons-material/Group";
import { type FC } from "react";
import { type ClassName, useClassName } from "hooks/useClassName";
import { Avatar } from "components/Avatar/Avatar";

export interface GroupAvatarProps {
  name: string;
  avatarURL?: string;
}

export const GroupAvatar: FC<GroupAvatarProps> = ({ name, avatarURL }) => {
  const badge = useClassName(classNames.badge, []);

  return (
    <Badge
      overlap="circular"
      anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
      badgeContent={<Group />}
      classes={{ badge }}
    >
      <Avatar src={avatarURL} background>
        {name}
      </Avatar>
    </Badge>
  );
};

const classNames = {
  badge: (css, theme) =>
    css({
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
    }),
} satisfies Record<string, ClassName>;
