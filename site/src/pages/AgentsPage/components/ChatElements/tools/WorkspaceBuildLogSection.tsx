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

// How long to wait for the first log entry before showing a
// warning. Builds can stay queued or run slow Terraform init for
// longer than this. The message is intentionally soft.
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

	// Only stream from a build that is actually in progress.
	// When the workspace query returns cached data from a previous
	// (e.g. stop) build, liveBuildId would point at the wrong build.
	// Treating it as undefined lets the component show the loading
	// state until the poll picks up the new start build.
	const latestBuildStatus = workspaceQuery.data?.latest_build?.status;
	const activeBuildId =
		latestBuildStatus === "pending" ||
		latestBuildStatus === "starting" ||
		latestBuildStatus === "running"
			? liveBuildId
			: undefined;

	// Use the active build ID while running, the result build ID
	// when completed.
	const effectiveBuildId = isRunning ? activeBuildId : buildId;

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
		: // Fall back to accumulated streaming logs while the REST
			// fetch is in-flight, avoiding a flash of "Loading…" on
			// the running→completed transition.
			(completedLogsQuery.data ?? streamingLogs);

	// --- Timeout: detect if logs never arrive ---
	// Derive a stable boolean so the effect only re-runs when logs
	// first appear or when the build ID changes, not on every
	// appended log entry.
	const hasLogs = Boolean(logs && logs.length > 0);
	const [timedOut, setTimedOut] = useState(false);
	useEffect(() => {
		setTimedOut(false);
		if (!effectiveBuildId || hasLogs) {
			return;
		}
		const timer = setTimeout(() => setTimedOut(true), LOG_LOAD_TIMEOUT_MS);
		return () => clearTimeout(timer);
	}, [effectiveBuildId, hasLogs]);

	const fetchFailed = !isRunning && completedLogsQuery.isError;

	if (!effectiveBuildId) {
		return null;
	}

	if (fetchFailed) {
		return (
			<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
				<TriangleAlertIcon className="h-3 w-3" />
				<span>Failed to load build logs.</span>
			</div>
		);
	}

	if (timedOut) {
		return (
			<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
				<TriangleAlertIcon className="h-3 w-3" />
				<span>Build logs are taking longer than expected.</span>
			</div>
		);
	}

	if (!logs || logs.length === 0) {
		return (
			<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
				<LoaderIcon className="h-3 w-3 animate-spin motion-reduce:animate-none" />
				<span>Loading build logs…</span>
			</div>
		);
	}

	return (
		<ScrollArea
			className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
			viewportClassName="max-h-64"
			scrollBarClassName="w-1.5"
		>
			<WorkspaceBuildLogs
				logs={logs}
				sticky
				className="border-0 rounded-none"
			/>
		</ScrollArea>
	);
};
