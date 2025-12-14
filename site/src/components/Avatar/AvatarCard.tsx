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
	return (
		<div
			className={cn(
				"flex flex-row flex-nowrap items-center gap-4",
				"p-4 rounded-lg cursor-default",
				"border border-solid border-zinc-200 dark:border-zinc-700",
			)}
			// TODO: We don't actually use this prop, so we should remove it.
			style={{
				...(maxWidth !== "none" ? { maxWidth: `${maxWidth}px` } : {}),
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
					className="leading-[1.4] m-0 overflow-hidden whitespace-nowrap text-ellipsis font-normal text-base"
				>
					{header}
				</h3>

				{subtitle && (
					<div className="text-sm leading-relaxed text-zinc-600 dark:text-zinc-400">
						{subtitle}
					</div>
				)}
			</div>

			<Avatar size="lg" src={imgUrl} fallback={header} />
		</div>
	);
};
