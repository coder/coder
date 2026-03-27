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
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

export const ToolIcon: React.FC<{
	name: string;
	isError: boolean;
	iconUrl?: string;
	isRunning?: boolean;
	serverName?: string;
}> = ({ name, iconUrl, isRunning, serverName }) => {
	const [imgError, setImgError] = useState(false);
	const color = "text-content-secondary";
	const base = cn("h-4 w-4 shrink-0", color, isRunning && "grayscale");

	// If an MCP icon URL is provided and hasn't failed, render it.
	// Strip colour so external icons match the monochrome lucide
	// style. brightness-0 forces every pixel to black, then in dark
	// mode we invert to white and tune opacity to approximate
	// content-secondary (light ≈ 34% lightness, dark ≈ 65%).
	if (iconUrl && !imgError) {
		const img = (
			<div className="h-4 w-4 shrink-0 overflow-hidden">
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className={cn(
						"block h-4 w-4",
						// Monochrome: brightness-0 strips colour to black,
						// dark:invert flips to white for dark backgrounds,
						// opacity tuned per-theme to match content-secondary
						// (light ~35% lightness, dark ~65%).
						"brightness-0 opacity-[0.35] dark:invert dark:opacity-[0.65]",
					)}
					onError={() => setImgError(true)}
				/>
			</div>
		);

		if (serverName) {
			return (
				<Tooltip>
					<TooltipTrigger asChild>{img}</TooltipTrigger>
					<TooltipContent>{serverName}</TooltipContent>
				</Tooltip>
			);
		}

		return img;
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
		case "spawn_computer_use_agent":
			return <MonitorIcon className={base} />;
		default:
			return <WrenchIcon className={base} />;
	}
};
