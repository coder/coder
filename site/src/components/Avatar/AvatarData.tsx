import { Avatar } from "components/Avatar/Avatar";
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
		<div className="flex items-center w-full gap-3">
			{avatar}

			<div className="flex flex-col w-full">
				<span className="text-sm font-semibold text-content-primary">
					{title}
				</span>
				{subtitle && (
					<span className="text-content-secondary text-xs font-medium">
						{subtitle}
					</span>
				)}
			</div>
		</div>
	);
};
