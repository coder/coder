import { cn } from "cn";
import { FolderIcon, FolderOpenIcon } from "lucide-react";
import type { FC } from "react";
import type { ChatProject } from "#/api/typesGenerated";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

type ChatProjectIconProps = {
	readonly project: Pick<ChatProject, "icon">;
	readonly expanded?: boolean;
	readonly className?: string;
};

/** The project's chosen icon, or a folder glyph when none is set. */
export const ChatProjectIcon: FC<ChatProjectIconProps> = ({
	project,
	expanded = false,
	className,
}) => {
	if (project.icon) {
		return (
			<ExternalImage
				src={project.icon}
				alt=""
				className={cn("shrink-0 object-contain", className)}
			/>
		);
	}
	const Glyph = expanded ? FolderOpenIcon : FolderIcon;
	return <Glyph aria-hidden="true" className={cn("shrink-0", className)} />;
};
