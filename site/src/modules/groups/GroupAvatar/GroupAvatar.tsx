import { Avatar } from "components/Avatar/Avatar";
import type { FC } from "react";

export interface GroupAvatarProps {
	name: string;
	avatarURL?: string;
}

export const GroupAvatar: FC<GroupAvatarProps> = ({ name, avatarURL }) => {
	return <Avatar src={avatarURL} fallback={name} />;
};
