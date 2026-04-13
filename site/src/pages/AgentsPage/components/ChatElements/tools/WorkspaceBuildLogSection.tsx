import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { workspaceBuildLogs } from "#/api/queries/workspaceBuilds";
import { workspaceById } from "#/api/queries/workspaces";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useWorkspaceBuildLogs } from "#/hooks/useWorkspaceBuildLogs";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import {
	useChatBuildId,
	useChatWorkspaceId,
} from "../../../context/ChatWorkspaceContext";
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
 * from the build tracked by the chat binding (or the workspace's
 * latest active build as a fallback). Once completed, logs
 * are fetched via REST and cached by React Query so expand/collapse
 * cycles don't re-fetch.
 */
export const WorkspaceBuildLogSection: FC<WorkspaceBuildLogSectionProps> = ({
	status,
	buildId,
}) => {
	const isRunning = status === "running";

	// Primary source: build ID from the chat binding, pushed via
	// pubsub when create_workspace or start_workspace persists it.
	// This avoids the 2s polling latency.
	const chatBuildId = useChatBuildId();

	// Fallback: poll the workspace to infer the build ID from
	// latest_build. Only used when the binding hasn't arrived yet.
	const workspaceId = useChatWorkspaceId();
	const needsPoll = isRunning && !chatBuildId;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: needsPoll && Boolean(workspaceId),
		refetchInterval: needsPoll ? 2000 : false,
	});
	const liveBuildId = workspaceQuery.data?.latest_build?.id;

	// Only use the polled build if it's actually in progress.
	const latestBuildStatus = workspaceQuery.data?.latest_build?.status;
	const polledActiveBuildId =
		latestBuildStatus === "pending" || latestBuildStatus === "starting"
			? liveBuildId
			: undefined;

	// While running: prefer chat binding (instant via pubsub),
	// fall back to polled workspace (2s latency). When completed:
	// use the build ID from the tool result.
	const effectiveBuildId = isRunning
		? (chatBuildId ?? polledActiveBuildId)
		: buildId;

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
		if (isRunning && workspaceId) {
			return (
				<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
					<LoaderIcon className="h-3 w-3 animate-spin motion-reduce:animate-none" />
					<span>Loading build logs…</span>
				</div>
			);
		}
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

	if (timedOut && !hasLogs) {
		return (
			<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
				<TriangleAlertIcon className="h-3 w-3" />
				<span>Build logs are taking longer than expected.</span>
			</div>
		);
	}

	// Query succeeded but the build produced no log output.
	if (
		!isRunning &&
		completedLogsQuery.isSuccess &&
		(!logs || logs.length === 0)
	) {
		return (
			<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
				<span>No build logs available.</span>
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
