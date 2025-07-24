import type { Workspace } from "api/typesGenerated";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { FC } from "react";
import type React from "react";
import {
	type DisplayWorkspaceStatusType,
	getDisplayWorkspaceStatus,
} from "utils/workspace";

const variantByStatusType: Record<
	DisplayWorkspaceStatusType,
	StatusIndicatorProps["variant"]
> = {
	active: "pending",
	inactive: "inactive",
	success: "success",
	error: "failed",
	danger: "warning",
	warning: "warning",
};

type WorkspaceStatusIndicatorProps = {
	workspace: Workspace;
	children?: React.ReactNode;
};

export const WorkspaceStatusIndicator: FC<WorkspaceStatusIndicatorProps> = ({
	workspace,
	children,
}) => {
	let { text, type } = getDisplayWorkspaceStatus(
		workspace.latest_build.status,
		workspace.latest_build.job,
		workspace,
	);

	if (!workspace.health.healthy) {
		type = "warning";
	}

	// Check if workspace is running but agents are still starting
	const isStarting =
		workspace.latest_build.status === "running" &&
		workspace.latest_build.resources.some((resource) =>
			resource.agents?.some(
				(agent) =>
					agent.lifecycle_state === "starting" ||
					agent.lifecycle_state === "created",
			),
		);

	const statusIndicator = (
		<StatusIndicator variant={variantByStatusType[type]}>
			<StatusIndicatorDot />
			<span className="sr-only">Workspace status:</span> {text}
			{children}
		</StatusIndicator>
	);

	// Show tooltip for unhealthy or starting workspaces
	if (!workspace.health.healthy || isStarting) {
		const tooltipMessage = !workspace.health.healthy
			? "Your workspace is running but some agents are unhealthy."
			: "Your workspace is running but startup scripts are still executing.";

		return (
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<StatusIndicator variant={variantByStatusType[type]}>
							<StatusIndicatorDot />
							<span className="sr-only">Workspace status:</span> {text}
							{children}
						</StatusIndicator>
					</TooltipTrigger>
					<TooltipContent>{tooltipMessage}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return statusIndicator;
};
