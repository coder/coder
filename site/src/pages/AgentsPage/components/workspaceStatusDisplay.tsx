import {
	MonitorDotIcon,
	MonitorIcon,
	MonitorPauseIcon,
	MonitorXIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import {
	type DisplayWorkspaceStatusType,
	getDisplayWorkspaceStatus,
} from "#/utils/workspace";

const iconCls = "size-3";

const statusIconMap: Record<DisplayWorkspaceStatusType, ReactNode> = {
	success: <MonitorIcon className={iconCls} />,
	active: <MonitorDotIcon className={iconCls} />,
	inactive: <MonitorPauseIcon className={iconCls} />,
	error: <MonitorXIcon className={iconCls} />,
	danger: <MonitorXIcon className={iconCls} />,
	warning: <MonitorXIcon className={iconCls} />,
};

interface WorkspaceStatusDisplay {
	statusLabel: string;
	statusIcon: ReactNode;
}

export function getWorkspaceStatusDisplay(
	workspace: Workspace,
	agent?: WorkspaceAgent | null,
): WorkspaceStatusDisplay {
	let { type, text } = getDisplayWorkspaceStatus(
		workspace.latest_build.status,
		workspace.latest_build.job,
	);

	// Override status when the workspace build is running but
	// the agent is still preparing or failed to start.
	const agentPreparing =
		workspace.latest_build.status === "running" &&
		(agent?.lifecycle_state === "created" ||
			agent?.lifecycle_state === "starting");
	const agentStartupFailed =
		workspace.latest_build.status === "running" &&
		(agent?.lifecycle_state === "start_error" ||
			agent?.lifecycle_state === "start_timeout");
	if (agentPreparing) {
		type = "active";
		text = "Preparing";
	} else if (agentStartupFailed) {
		type = "warning";
		text = "Startup failed";
	}

	const effectiveType = workspace.health.healthy ? type : "warning";
	const statusLabel = workspace.health.healthy
		? `Workspace ${text.toLowerCase()}`
		: `Workspace ${text.toLowerCase()} (unhealthy)`;
	return {
		statusLabel,
		statusIcon: statusIconMap[effectiveType],
	};
}
