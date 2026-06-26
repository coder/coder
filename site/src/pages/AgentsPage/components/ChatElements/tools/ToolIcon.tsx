import {
	BadgeQuestionMarkIcon,
	BotIcon,
	CompassIcon,
	FilePenLineIcon,
	FileTextIcon,
	LightbulbIcon,
	LogInIcon,
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
import {
	isSubagentToolName,
	type SubagentIconKind,
} from "./subagentDescriptor";

export const ToolIcon: React.FC<{
	name: string;
	isError: boolean;
	iconUrl?: string;
	isRunning?: boolean;
	serverName?: string;
	subagentIconKind?: SubagentIconKind;
}> = ({ name, iconUrl, isRunning, serverName, subagentIconKind }) => {
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

	if (isSubagentToolName(name)) {
		// This name-based fallback only exists for legacy callers that do
		// not pass a descriptor. The descriptor path should provide
		// subagentIconKind for new subagent types instead of extending it.
		const iconKind =
			subagentIconKind ||
			(name === "spawn_computer_use_agent" ? "monitor" : "bot");
		return iconKind === "monitor" ? (
			<MonitorIcon className={base} />
		) : (
			<BotIcon className={base} />
		);
	}

	switch (name) {
		case "execute":
		case "process_output":
		case "process_list":
		case "process_signal":
			return <TerminalIcon className={base} />;
		case "wait_for_external_auth":
			return <LogInIcon className={base} />;
		case "read_file":
		case "read_skill":
		case "read_skill_file":
			return <FileTextIcon className={base} />;
		case "write_file":
		case "edit_files":
			return <FilePenLineIcon className={base} />;
		case "list_templates":
		case "read_template":
		case "create_workspace":
			return <ServerIcon className={base} />;
		case "start_workspace":
			return <PowerIcon className={base} />;
		case "chat_summarized":
			return <BotIcon className={base} />;
		case "thinking":
			return <LightbulbIcon className={base} />;
		case "propose_plan":
			return <RouteIcon className={base} />;
		case "ask_user_question":
			return <BadgeQuestionMarkIcon className={base} />;
		case "advisor":
			return <CompassIcon className={base} />;
		case "computer":
			return <MonitorIcon className={base} />;

		default:
			return <WrenchIcon className={base} />;
	}
};
