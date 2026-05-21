import type { FC, ReactNode } from "react";
import { Avatar } from "#/components/Avatar/Avatar";

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
}

export const AvatarData: FC<AvatarDataProps> = ({
	title,
	subtitle,
	src,
	imgFallbackText,
	avatar,
}) => {
	if (!avatar) {
		avatar = (
			<Avatar
				size="lg"
				src={src}
				fallback={(typeof title === "string" ? title : imgFallbackText) || "-"}
			/>
		);
	}

	return (
		<div className="flex min-w-0 items-center gap-3">
			{avatar}

			<div className="flex min-w-0 flex-1 flex-col overflow-hidden">
				<span className="truncate text-sm font-semibold text-content-primary">
					{title}
				</span>
				{subtitle && (
					<span className="truncate text-content-secondary text-xs font-medium">
						{subtitle}
					</span>
				)}
			</div>
		</div>
	);
};
