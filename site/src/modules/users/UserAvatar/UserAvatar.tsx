import { Avatar, type AvatarProps } from "components/Avatar/Avatar";
import type { FC } from "react";

export type UserAvatarProps = {
	username: string;
	avatarURL?: string;
	size?: AvatarProps["size"];
};

export const UserAvatar: FC<UserAvatarProps> = ({
	username,
	avatarURL,
	size,
}) => {
	return <Avatar size={size} src={avatarURL} fallback={username} />;
};
