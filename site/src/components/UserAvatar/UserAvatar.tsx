import type { FC } from "react";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";

export type UserAvatarProps = {
  username: string;
  avatarURL?: string;
} & AvatarProps;

export const UserAvatar: FC<UserAvatarProps> = ({
  username,
  avatarURL,
  ...avatarProps
}) => {
  return (
    <Avatar background title={username} src={avatarURL} {...avatarProps}>
      {username}
    </Avatar>
  );
};
