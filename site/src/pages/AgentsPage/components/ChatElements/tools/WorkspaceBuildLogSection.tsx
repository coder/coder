import { LoaderIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { workspaceById } from "#/api/queries/workspaces";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useWorkspaceBuildLogs } from "#/hooks/useWorkspaceBuildLogs";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { useChatWorkspaceId } from "../../../context/ChatWorkspaceContext";
import type { ToolStatus } from "./utils";

interface WorkspaceBuildLogSectionProps {
	status: ToolStatus;
	/** Build ID from the completed tool result. */
	buildId?: string;
}

/**
 * Streams or fetches workspace build logs for display inside a tool
 * collapsible. While the tool is running, logs stream from the
 * workspace's current latest build. Once completed, uses the build ID
 * from the tool result to load historical logs on demand.
 */
export const WorkspaceBuildLogSection: FC<WorkspaceBuildLogSectionProps> = ({
	status,
	buildId,
}) => {
	const isRunning = status === "running";

	// During execution, get the live build ID from the workspace.
	const workspaceId = useChatWorkspaceId();
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: isRunning && Boolean(workspaceId),
	});
	const liveBuildId = workspaceQuery.data?.latest_build?.id;

	// Use the live build ID while running, the result build ID when
	// completed.
	const effectiveBuildId = isRunning ? liveBuildId : buildId;

	// For completed tools, only connect when the section is mounted
	// (parent controls visibility via expand/collapse).
	const logs = useWorkspaceBuildLogs(
		effectiveBuildId,
		Boolean(effectiveBuildId),
	);

	if (!effectiveBuildId) {
		return null;
	}

	if (!logs || logs.length === 0) {
		return (
			<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
				<LoaderIcon className="h-3 w-3 animate-spin" />
				<span>Loading build logs…</span>
			</div>
		);
	}

	return (
		<ScrollArea className="max-h-96 overflow-auto mt-2">
			<WorkspaceBuildLogs
				logs={logs}
				sticky
				className="border-0 rounded-none text-xs"
			/>
		</ScrollArea>
	);
};
