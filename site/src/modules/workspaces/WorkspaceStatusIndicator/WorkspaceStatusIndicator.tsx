import type { Workspace } from "api/typesGenerated";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
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
	const { text, type } = getDisplayWorkspaceStatus(
		workspace.latest_build.status,
		workspace.latest_build.job,
	);

	return (
		<StatusIndicator variant={variantByStatusType[type]}>
			<StatusIndicatorDot />
			<span>
				<span className="sr-only">Workspace status:</span> {text}
			</span>
			{children}
		</StatusIndicator>
	);
};
