import Badge from "@mui/material/Badge";
import Group from "@mui/icons-material/Group";
import { type FC } from "react";
import { Avatar } from "components/Avatar/Avatar";
import { makeClassNames } from "hooks/useClassNames";

export interface GroupAvatarProps {
  name: string;
  avatarURL?: string;
}

export const GroupAvatar: FC<GroupAvatarProps> = ({ name, avatarURL }) => {
  const classNames = useClassNames(null);

  return (
    <Badge
      overlap="circular"
      anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
      badgeContent={<Group />}
      classes={{ badge: classNames.badge }}
    >
      <Avatar background src={avatarURL}>
        {name}
      </Avatar>
    </Badge>
  );
};

const useClassNames = makeClassNames((css, theme) => ({
  badge: css({
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
}));
