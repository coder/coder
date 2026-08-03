import type { FC } from "react";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";

interface StartWorkspaceToolProps {
	status: ToolStatus;
	buildId?: string;
	workspaceName: string;
	errorMessage?: string;
	noBuild?: boolean;
	labelOverride?: string;
}

export const StartWorkspaceTool: FC<StartWorkspaceToolProps> = ({
	status,
	buildId,
	workspaceName,
	errorMessage,
	noBuild,
	labelOverride,
}) => {
	const isRunning = status === "running";
	const isError = status === "error";

	const label = isRunning
		? "Starting workspace…"
		: labelOverride
			? labelOverride
			: isError
				? `Failed to start ${workspaceName || "workspace"}`
				: workspaceName
					? `Started ${workspaceName}`
					: "Started workspace";

	const hasBuildLogs = (isRunning || Boolean(buildId)) && !noBuild;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			errorMessage={errorMessage || "Failed to start workspace"}
			hasContent={hasBuildLogs}
			defaultExpanded={isRunning}
		>
			<ToolCall.Header iconName="start_workspace" label={label} />
			<ToolCall.Content>
				<WorkspaceBuildLogSection status={status} buildId={buildId} />
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
