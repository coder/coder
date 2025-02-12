import { useTheme } from "@emotion/react";
import type { WorkspaceBuild } from "api/typesGenerated";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";
import { BuildIcon } from "components/BuildIcon/BuildIcon";
import { useClassName } from "hooks/useClassName";
import type { FC } from "react";
import { getDisplayWorkspaceBuildStatus } from "utils/workspace";

export interface BuildAvatarProps {
	build: WorkspaceBuild;
	size?: AvatarProps["size"];
}

export const BuildAvatar: FC<BuildAvatarProps> = ({ build, size }) => {
	const theme = useTheme();
	const { type } = getDisplayWorkspaceBuildStatus(theme, build);
	const iconColor = useClassName(
		(css, theme) => css({ color: theme.roles[type].fill.solid }),
		[type],
	);

	return (
		<Avatar size={size} variant="icon">
			<BuildIcon
				transition={build.transition}
				className={`w-full h-full ${iconColor}`}
			/>
		</Avatar>
	);
};
