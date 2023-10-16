import { Avatar, AvatarProps } from "components/Avatar/Avatar";
import { FC } from "react";

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
    <Avatar title={username} src={avatarURL} {...avatarProps}>
      {username}
    </Avatar>
  );
};
