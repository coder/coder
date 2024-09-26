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
	 * Useful for when you need to pass in a ReactNode for the title of the
	 * component.
	 *
	 * MUI will try to take any string titles and turn them into the first
	 * character, but if you pass in a ReactNode, MUI can't do that, because it
	 * has no way to reliably grab the text content during renders.
	 *
	 * Tried writing some layout effect/JSX parsing logic to do the extraction,
	 * but it added complexity and render overhead, and wasn't reliable enough
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
	avatar ??= (
		<Avatar background src={src}>
			{typeof title === "string" ? title : (imgFallbackText ?? "-")}
		</Avatar>
	);

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
