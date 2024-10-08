import { useTheme } from "@emotion/react";
import { Avatar } from "components/Avatar/Avatar";
import { Stack } from "components/Stack/Stack";
import type { FC, ReactNode } from "react";

export interface AvatarDataProps {
	title: ReactNode;
	subtitle?: ReactNode;
	src?: string;
	avatar?: React.ReactNode;

	/**
	 * Lets you specify the character(s) displayed in an avatar when an image is
	 * unavailable (like when the network request fails).
	 *
	 * If not specified, the component will try to parse the first character
	 * from the title prop if it is a string.
	 */
	imgFallbackText?: string;
}

export const AvatarData: FC<AvatarDataProps> = ({
	title,
	subtitle,
	src,
	imgFallbackText,
	avatar,
}) => {
	const theme = useTheme();
	if (!avatar) {
		avatar = (
			<Avatar background src={src}>
				{(typeof title === "string" ? title : imgFallbackText) || "-"}
			</Avatar>
		);
	}

	return (
		<Stack
			spacing={1.5}
			direction="row"
			alignItems="center"
			css={{
				minHeight: 40, // Make it predictable for the skeleton
				width: "100%",
				lineHeight: "150%",
			}}
		>
			{avatar}

			<Stack
				spacing={0}
				css={{
					width: "100%",
				}}
			>
				<span
					css={{
						color: theme.palette.text.primary,
						fontWeight: 600,
					}}
				>
					{title}
				</span>
				{subtitle && (
					<span
						css={{
							fontSize: 13,
							color: theme.palette.text.secondary,
							lineHeight: 1.5,
							maxWidth: 540,
						}}
					>
						{subtitle}
					</span>
				)}
			</Stack>
		</Stack>
	);
};
