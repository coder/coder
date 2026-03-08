import type { Workspace, WorkspaceTransition } from "api/typesGenerated";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type React from "react";
import type { FC } from "react";
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

const failedTransitionTooltips: Record<WorkspaceTransition, string> = {
	start: "Build failed during start",
	stop: "Build failed during stop",
	delete: "Build failed during deletion",
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
	);

	if (!workspace.health.healthy) {
		type = "warning";
	}

	const isFailed = workspace.latest_build.status === "failed";
	const isUnhealthy = !workspace.health.healthy;

	// Determine tooltip text based on workspace state
	let tooltipText: string | undefined;
	if (isFailed) {
		tooltipText =
			failedTransitionTooltips[workspace.latest_build.transition];
	} else if (isUnhealthy) {
		tooltipText =
			"Your workspace is running but some agents are unhealthy.";
	}

	const statusIndicatorContent = (
		<StatusIndicator variant={variantByStatusType[type]}>
			<StatusIndicatorDot />
			<span className="sr-only">Workspace status:</span> {text}
			{children}
		</StatusIndicator>
	);

	if (!tooltipText) {
		return statusIndicatorContent;
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				{statusIndicatorContent}
			</TooltipTrigger>
			<TooltipContent>{tooltipText}</TooltipContent>
		</Tooltip>
	);
};
