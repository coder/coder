import Group from "@mui/icons-material/Group";
import Badge from "@mui/material/Badge";
import type { FC } from "react";
import { Avatar } from "components/Avatar/Avatar";
import { type ClassName, useClassName } from "hooks/useClassName";

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
      <Avatar background src={avatarURL}>
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
