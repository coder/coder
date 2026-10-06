import { cn } from "cn";
import { FolderIcon, FolderOpenIcon } from "lucide-react";
import { useState } from "react";
import type { ChatProject } from "#/api/typesGenerated";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

type ChatProjectIconProps = {
	readonly project: Pick<ChatProject, "icon">;
	readonly expanded?: boolean;
	readonly className?: string;
};

/**
 * The project's icon, or a folder glyph when none is set or it fails to load.
 * `expanded` only changes the folder glyph.
 */
export const ChatProjectIcon: React.FC<ChatProjectIconProps> = ({
	project,
	expanded = false,
	className,
}) => {
	// Remembers which URL failed, so a new icon gets a fresh attempt.
	const [failedIcon, setFailedIcon] = useState<string>();

	if (project.icon && project.icon !== failedIcon) {
		const icon = project.icon;
		return (
			<ExternalImage
				src={icon}
				alt=""
				className={cn("shrink-0 object-contain", className)}
				onError={() => setFailedIcon(icon)}
			/>
		);
	}
	const Glyph = expanded ? FolderOpenIcon : FolderIcon;
	return <Glyph aria-hidden="true" className={cn("shrink-0", className)} />;
};
