import {
	BotIcon,
	ClipboardListIcon,
	FileIcon,
	FilePenIcon,
	MonitorIcon,
	PlusCircleIcon,
	TerminalIcon,
	WrenchIcon,
} from "lucide-react";
import type React from "react";
import { useState } from "react";
import { cn } from "utils/cn";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

export const ToolIcon: React.FC<{
	name: string;
	isError: boolean;
	iconUrl?: string;
	isRunning?: boolean;
}> = ({ name, isError, iconUrl, isRunning }) => {
	const [imgError, setImgError] = useState(false);
	const color = isError ? "text-content-destructive" : "text-content-secondary";
	const base = cn("h-4 w-4 shrink-0", color, isRunning && "grayscale");

	// If an MCP icon URL is provided and hasn't failed, render it.
	// External images can't be recolored to an exact CSS token, so
	// on error the image is shown with reduced opacity. The error
	// state is communicated by the red label text and alert icon.
	if (iconUrl && !imgError) {
		return (
			<ExternalImage
				src={iconUrl}
				alt={`${name} icon`}
				className={cn(
					"block h-4 w-4 shrink-0",
					isError && "opacity-50 grayscale",
					isRunning && "grayscale",
				)}
				onError={() => setImgError(true)}
			/>
		);
	}

	switch (name) {
		case "execute":
		case "process_output":
			return <TerminalIcon className={base} />;
		case "read_file":
		case "list_templates":
		case "read_template":
			return <FileIcon className={base} />;
		case "write_file":
		case "edit_files":
			return <FilePenIcon className={base} />;
		case "create_workspace":
			return <PlusCircleIcon className={base} />;
		case "chat_summarized":
			return <BotIcon className={base} />;
		case "propose_plan":
			return <ClipboardListIcon className={base} />;
		case "computer":
			return <MonitorIcon className={base} />;
		default:
			return <WrenchIcon className={base} />;
	}
};
