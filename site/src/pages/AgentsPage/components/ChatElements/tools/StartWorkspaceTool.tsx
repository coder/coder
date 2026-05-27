import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { ToolCollapsible } from "./ToolCollapsible";
import { ToolIcon } from "./ToolIcon";
import type { ToolStatus } from "./utils";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";

interface StartWorkspaceToolProps {
	status: ToolStatus;
	buildId?: string;
	workspaceName: string;
	isError: boolean;
	errorMessage?: string;
	noBuild?: boolean;
	labelOverride?: string;
}

export const StartWorkspaceTool: FC<StartWorkspaceToolProps> = ({
	status,
	buildId,
	workspaceName,
	isError,
	errorMessage,
	noBuild,
	labelOverride,
}) => {
	const isRunning = status === "running";

	const label = isRunning
		? "Starting workspace…"
		: labelOverride
			? labelOverride
			: isError
				? `Failed to start ${workspaceName || "workspace"}`
				: workspaceName
					? `Started ${workspaceName}`
					: "Started workspace";

	const header = (
		<>
			<ToolIcon
				name="start_workspace"
				isError={isError}
				isRunning={isRunning}
			/>
			<span className="text-[13px] leading-6">{label}</span>
		</>
	);
	const headerStatus = (
		<>
			{isError && (
				<Tooltip>
					<TooltipTrigger asChild>
						<TriangleAlertIcon className="size-3.5 shrink-0 text-current" />
					</TooltipTrigger>
					<TooltipContent>
						{errorMessage || "Failed to start workspace"}
					</TooltipContent>
				</Tooltip>
			)}
			{isRunning && (
				<LoaderIcon className="size-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
			)}
		</>
	);

	// Show collapsible with build logs when there's a build to show.
	const hasBuildLogs = (isRunning || Boolean(buildId)) && !noBuild;

	return (
		<div className="w-full">
			<ToolCollapsible
				header={header}
				headerStatus={headerStatus}
				hasContent={hasBuildLogs}
				defaultExpanded={isRunning}
			>
				<WorkspaceBuildLogSection status={status} buildId={buildId} />
			</ToolCollapsible>
		</div>
	);
};
