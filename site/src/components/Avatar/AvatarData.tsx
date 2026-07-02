import type { FC, ReactNode } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import { cn } from "#/utils/cn";

interface AvatarDataProps {
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

	alt?: string;

	/**
	 * When true, the title and subtitle clip with an ellipsis if they overflow
	 * the available width. Off by default because callers that pass non-text
	 * nodes (icons, badges) as `title` would otherwise clip silently.
	 */
	truncate?: boolean;
}

export const AvatarData: FC<AvatarDataProps> = ({
	title,
	subtitle,
	src,
	imgFallbackText,
	avatar,
	alt = "",
	truncate = false,
}) => {
	if (!avatar) {
		avatar = (
			<Avatar
				size="lg"
				src={src}
				fallback={(typeof title === "string" ? title : imgFallbackText) || "-"}
				alt={alt}
			/>
		);
	}

	return (
		<div className="flex items-center gap-3">
			{avatar}

			<div
				className={cn("flex flex-col", truncate && "flex-1 overflow-hidden")}
			>
				<span
					className={cn(
						"text-sm font-semibold text-content-primary",
						truncate && "truncate",
					)}
				>
					{title}
				</span>
				{subtitle && (
					<span
						className={cn(
							"text-content-secondary text-xs font-medium",
							truncate && "truncate",
						)}
					>
						{subtitle}
					</span>
				)}
			</div>
		</div>
	);
};
