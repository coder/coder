import {
	Avatar,
	AvatarFallback,
	AvatarImage,
	avatarLetter,
} from "components/Avatar/Avatar";
import type { FC } from "react";

export interface GroupAvatarProps {
	name: string;
	avatarURL?: string;
}

export const GroupAvatar: FC<GroupAvatarProps> = ({ name, avatarURL }) => {
	return (
		<Avatar>
			<AvatarImage src={avatarURL} />
			<AvatarFallback>{avatarLetter(name)}</AvatarFallback>
		</Avatar>
	);
};
