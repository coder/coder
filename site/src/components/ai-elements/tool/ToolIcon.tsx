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
}> = ({ name, isError, iconUrl }) => {
	const [imgError, setImgError] = useState(false);
	const color = isError ? "text-content-destructive" : "text-content-secondary";
	const base = cn("h-4 w-4 shrink-0", color);

	// If an MCP icon URL is provided and hasn't failed, render it.
	if (iconUrl && !imgError) {
		return (
			<div
				className={cn(
					"flex h-4 w-4 shrink-0 items-center justify-center",
					"rounded-full bg-surface-secondary",
					isError && "ring-1 ring-content-destructive",
				)}
			>
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className="h-3 w-3"
					onError={() => setImgError(true)}
				/>
			</div>
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
