import { LoaderIcon, PlayIcon, TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { ToolCollapsible } from "./ToolCollapsible";
import type { ToolStatus } from "./utils";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";

interface StartWorkspaceToolProps {
	status: ToolStatus;
	buildId?: string;
	workspaceName: string;
	isError: boolean;
	errorMessage?: string;
}

export const StartWorkspaceTool: FC<StartWorkspaceToolProps> = ({
	status,
	buildId,
	workspaceName,
	isError,
	errorMessage,
}) => {
	const isRunning = status === "running";

	const label = isRunning
		? "Starting workspace…"
		: workspaceName
			? `Started ${workspaceName}`
			: "Started workspace";

	const header = (
		<>
			<PlayIcon className="h-4 w-4 shrink-0 text-content-secondary" />
			<span className={cn("text-sm", "text-content-secondary")}>{label}</span>
			{isError && (
				<Tooltip>
					<TooltipTrigger asChild>
						<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
					</TooltipTrigger>
					<TooltipContent>
						{errorMessage || "Failed to start workspace"}
					</TooltipContent>
				</Tooltip>
			)}
			{isRunning && (
				<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
			)}
		</>
	);

	// Show collapsible with build logs when there's a build to show.
	const hasBuildLogs = isRunning || Boolean(buildId);

	return (
		<div className="w-full">
			<ToolCollapsible
				header={header}
				hasContent={hasBuildLogs}
				defaultExpanded={isRunning}
			>
				<WorkspaceBuildLogSection status={status} buildId={buildId} />
			</ToolCollapsible>
		</div>
	);
};
