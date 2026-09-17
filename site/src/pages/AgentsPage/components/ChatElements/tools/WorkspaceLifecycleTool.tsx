import type { FC } from "react";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";

type WorkspaceLifecycleToolProps = {
	action: "start" | "stop";
	status: ToolStatus;
	buildId?: string;
	workspaceName: string;
	isError: boolean;
	errorMessage?: string;
	noBuild?: boolean;
	labelOverride?: string;
};

export const WorkspaceLifecycleTool: FC<WorkspaceLifecycleToolProps> = ({
	action,
	status,
	buildId,
	workspaceName,
	isError,
	errorMessage,
	noBuild,
	labelOverride,
}) => {
	const isRunning = status === "running";
	const completed = action === "start" ? "Started" : "Stopped";

	let label: string;
	if (isRunning) {
		label = action === "start" ? "Starting workspace…" : "Stopping workspace…";
	} else if (labelOverride) {
		label = labelOverride;
	} else if (isError) {
		label = `Failed to ${action} ${workspaceName || "workspace"}`;
	} else {
		label = `${completed} ${workspaceName || "workspace"}`;
	}

	const hasBuildLogs = (isRunning || Boolean(buildId)) && !noBuild;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || `Failed to ${action} workspace`}
			hasContent={hasBuildLogs}
			defaultExpanded={isRunning}
		>
			<ToolCall.Header iconName={`${action}_workspace`} label={label} />
			<ToolCall.Content>
				<WorkspaceBuildLogSection status={status} buildId={buildId} />
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
