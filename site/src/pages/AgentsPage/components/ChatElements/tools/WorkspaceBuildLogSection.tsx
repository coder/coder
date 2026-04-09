import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { workspaceBuildLogs } from "#/api/queries/workspaceBuilds";
import { workspaceById } from "#/api/queries/workspaces";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
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

// How long to wait for the first log entry before showing an error.
const LOG_LOAD_TIMEOUT_MS = 30_000;

/**
 * Streams or fetches workspace build logs for display inside a tool
 * collapsible. While the tool is running, logs stream via WebSocket
 * from the workspace's current latest build. Once completed, logs
 * are fetched via REST and cached by React Query so expand/collapse
 * cycles don't re-fetch.
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
		refetchInterval: isRunning ? 2000 : false,
	});
	const liveBuildId = workspaceQuery.data?.latest_build?.id;

	// Use the live build ID while running, the result build ID when
	// completed.
	const effectiveBuildId = isRunning ? liveBuildId : buildId;

	// --- Running builds: stream via WebSocket ---
	const streamingLogs = useWorkspaceBuildLogs(
		isRunning ? effectiveBuildId : undefined,
		isRunning && Boolean(effectiveBuildId),
	);

	// --- Completed builds: fetch via REST (cached across mounts) ---
	const completedLogsQuery = useQuery({
		...workspaceBuildLogs(effectiveBuildId ?? ""),
		enabled: !isRunning && Boolean(effectiveBuildId),
	});

	const logs: ProvisionerJobLog[] | undefined = isRunning
		? streamingLogs
		: completedLogsQuery.data;

	// --- Timeout: detect if logs never arrive ---
	const [timedOut, setTimedOut] = useState(false);
	useEffect(() => {
		if (!effectiveBuildId || (logs && logs.length > 0)) {
			setTimedOut(false);
			return;
		}
		const timer = setTimeout(() => setTimedOut(true), LOG_LOAD_TIMEOUT_MS);
		return () => clearTimeout(timer);
	}, [effectiveBuildId, logs]);

	// --- Error state ---
	const hasError = timedOut || (!isRunning && completedLogsQuery.isError);

	if (!effectiveBuildId) {
		return null;
	}

	if (hasError) {
		return (
			<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
				<TriangleAlertIcon className="h-3 w-3" />
				<span>Failed to load build logs.</span>
			</div>
		);
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
		<ScrollArea className="mt-2" viewportClassName="max-h-96">
			<WorkspaceBuildLogs
				logs={logs}
				sticky
				className="border-0 rounded-none text-xs"
			/>
		</ScrollArea>
	);
};
