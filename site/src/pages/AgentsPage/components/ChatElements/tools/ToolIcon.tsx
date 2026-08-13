import {
	BadgeQuestionMarkIcon,
	BotIcon,
	CompassIcon,
	FilePenLineIcon,
	FileTextIcon,
	LightbulbIcon,
	type LucideIcon,
	MonitorIcon,
	PowerIcon,
	RouteIcon,
	ServerIcon,
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

export const toolIcons: Partial<Record<string, LucideIcon>> = {
	execute: TerminalIcon,
	process_output: TerminalIcon,
	process_list: TerminalIcon,
	process_signal: TerminalIcon,
	read_file: FileTextIcon,
	read_skill: FileTextIcon,
	write_file: FilePenLineIcon,
	edit_files: FilePenLineIcon,
	list_templates: ServerIcon,
	read_template: ServerIcon,
	create_workspace: ServerIcon,
	start_workspace: PowerIcon,
	chat_cleared: BotIcon,
	chat_summarized: BotIcon,
	list_agents: BotIcon,
	list_subagent_models: BotIcon,
	thinking: LightbulbIcon,
	propose_plan: RouteIcon,
	ask_user_question: BadgeQuestionMarkIcon,
	advisor: CompassIcon,
	computer: MonitorIcon,
};

export const ToolIcon: React.FC<{
	name: string;
	iconUrl?: string;
	isRunning?: boolean;
	serverName?: string;
}> = ({ name, iconUrl, isRunning, serverName }) => {
	const [imgError, setImgError] = useState(false);
	const color = "text-current";
	const base = cn(
		"size-4 shrink-0",
		color,
		"stroke-[1.5]",
		isRunning && "grayscale",
	);

	// If an MCP icon URL is provided and hasn't failed, render it.
	// Strip colour so external icons match the monochrome lucide
	// style. brightness-0 forces every pixel to black, then in dark
	// mode we invert to white and tune opacity to approximate
	// content-secondary (light ≈ 34% lightness, dark ≈ 65%).
	if (iconUrl && !imgError) {
		const img = (
			<div className="size-4 shrink-0 overflow-hidden">
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className={cn(
						"block size-4",
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

	const Icon = toolIcons[name] ?? WrenchIcon;
	return <Icon className={base} />;
};
