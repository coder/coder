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
	// Strip colour so external icons match the monochrome lucide
	// style. brightness-0 forces every pixel to black, then in dark
	// mode we invert to white and tune opacity to approximate
	// content-secondary (light ≈ 34% lightness, dark ≈ 65%).
	// On error we halve opacity further; on running it's the same
	// monochrome treatment (already desaturated).
	if (iconUrl && !imgError) {
		return (
			<ExternalImage
				src={iconUrl}
				alt={`${name} icon`}
				className={cn(
					"block h-4 w-4 shrink-0 brightness-0 opacity-[0.35] dark:invert dark:opacity-[0.65]",
					isError && "!opacity-[0.2] dark:!opacity-[0.35]",
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
