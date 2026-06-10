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

	// External MCP icons render through the shared `ExternalImage` and
	// `getExternalImageStylesFromUrl` pipeline. Bundled `/icon/*.svg`
	// paths that opt in via `defaultParametersForBuiltinIcons` receive
	// the theme-aware `monochrome` filter (light:
	// `grayscale(100%) contrast(0%) brightness(70%)`, dark:
	// `... brightness(250%)`), which produces a uniform muted
	// silhouette next to the surrounding lucide icons. Cross-origin
	// URLs that aren't in the map fall through unfiltered, matching
	// the MCP server pill rendering.
	if (iconUrl && !imgError) {
		const img = (
			<ExternalImage
				src={iconUrl}
				alt={`${name} icon`}
				className={cn(
					"size-4 shrink-0 object-contain",
					isRunning && "grayscale",
				)}
				onError={() => setImgError(true)}
			/>
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
