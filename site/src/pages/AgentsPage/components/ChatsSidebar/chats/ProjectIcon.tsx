import { cn } from "cn";
import { FolderIcon } from "lucide-react";
import type { FC } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

type ProjectIconProps = {
	/** Emoji path or image URL; empty renders the default folder glyph. */
	readonly icon: string;
	readonly className?: string;
};

export const ProjectIcon: FC<ProjectIconProps> = ({ icon, className }) => {
	if (!icon) {
		return (
			<FolderIcon
				aria-hidden="true"
				className={cn("size-4 shrink-0", className)}
			/>
		);
	}
	return (
		<ExternalImage
			alt=""
			src={icon}
			className={cn("size-4 shrink-0 object-contain", className)}
		/>
	);
};
