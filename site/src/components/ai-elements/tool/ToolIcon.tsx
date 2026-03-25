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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export const ToolIcon: React.FC<{
	name: string;
	isError: boolean;
	iconUrl?: string;
	isRunning?: boolean;
	serverName?: string;
}> = ({ name, isError, iconUrl, isRunning, serverName }) => {
	const [imgError, setImgError] = useState(false);
	const color = isError ? "text-content-destructive" : "text-content-secondary";
	const base = cn("h-4 w-4 shrink-0", color, isRunning && "grayscale");

	// If an MCP icon URL is provided and hasn't failed, render it.
	// Strip colour so external icons match the monochrome lucide
	// style. brightness-0 forces every pixel to black, then in dark
	// mode we invert to white and tune opacity to approximate
	// content-secondary (light ≈ 34% lightness, dark ≈ 65%).
	if (iconUrl && !imgError) {
		// Always render the same DOM shape so React never unmounts
		// the <img> when isError changes (avoids a reload flicker).
		//
		// The wrapper clips a translated copy of the image so the
		// drop-shadow trick can work on error (see image classes).
		// In the normal state the image is not translated and the
		// wrapper's overflow-hidden is a harmless no-op.
		const img = (
			<div className="h-4 w-4 shrink-0 overflow-hidden">
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className={cn(
						"block h-4 w-4",
						isError
							? // Drop-shadow recolor: brightness-0 makes every
								// pixel black, drop-shadow casts an exact-color
								// copy 16px to the right following the alpha
								// channel, -translate-x-4 shifts the colored copy
								// into the original position, and the wrapper's
								// overflow-hidden clips the black original.
								"-translate-x-4 [filter:brightness(0)_drop-shadow(16px_0_0_hsl(var(--content-destructive)))]"
							: // Monochrome: brightness-0 strips colour to black,
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
			return <MonitorIcon className={base} />;
		default:
			return <WrenchIcon className={base} />;
	}
};
