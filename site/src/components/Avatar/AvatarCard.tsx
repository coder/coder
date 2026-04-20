import { type CSSObject, useTheme } from "@emotion/react";
import type { FC, ReactNode } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import { cn } from "#/utils/cn";

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
				"flex flex-row flex-nowrap gap-4 items-center",
				"border border-solid p-4 rounded-lg cursor-default",
			)}
			style={{
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
					className="text-base leading-snug m-0 truncate"
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
