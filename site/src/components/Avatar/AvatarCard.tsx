import { type CSSObject, useTheme } from "@emotion/react";
import { Avatar } from "components/Avatar/Avatar";
import type { FC, ReactNode } from "react";
import { cn } from "utils/cn";

type AvatarCardProps = {
	header: string;
	imgUrl: string;
	subtitle?: ReactNode;
	maxWidth?: number | "none";
};

export const AvatarCard: FC<AvatarCardProps> = ({
	header,
	imgUrl,
	subtitle,
	maxWidth = "none",
}) => {
	const theme = useTheme();

	return (
		<div
			className={cn(
				"flex flex-row flex-nowrap items-center gap-4",
				"p-4 rounded-lg cursor-default border border-solid border-zinc-700",
			)}
			// TODO: We don't actually use this prop, so we should remove it.
			css={{
				maxWidth: maxWidth === "none" ? undefined : `${maxWidth}px`,
			}}
		>
			{/**
			 * minWidth is necessary to ensure that the text truncation works properly
			 * with flex containers that don't have fixed width
			 *
			 * @see {@link https://css-tricks.com/flexbox-truncated-text/}
			 */}
			<div className="mr-auto min-w-0">
				<h3
					// Lets users hover over truncated text to see whole thing
					title={header}
					css={[theme.typography.body1 as CSSObject]}
					className="leading-[1.4] m-0 overflow-hidden whitespace-nowrap text-ellipsis"
				>
					{header}
				</h3>

				{subtitle && (
					<div
						css={[
							theme.typography.body2 as CSSObject,
							{ color: theme.palette.text.secondary },
						]}
					>
						{subtitle}
					</div>
				)}
			</div>

			<Avatar size="lg" src={imgUrl} fallback={header} />
		</div>
	);
};
