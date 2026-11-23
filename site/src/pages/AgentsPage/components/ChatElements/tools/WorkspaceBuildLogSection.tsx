import { useEffect, useState } from "react";
import { useQuery } from "react-query";
import {
	workspaceBuildById,
	workspaceBuildLogs,
} from "#/api/queries/workspaceBuilds";
import { workspaceById } from "#/api/queries/workspaces";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { useWorkspaceBuildLogs } from "#/hooks/useWorkspaceBuildLogs";
import { ACTIVE_BUILD_STATUSES } from "#/modules/workspaces/status";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { useChatWorkspace } from "../../../context/ChatWorkspaceContext";
import { LogNotice } from "./LogNotice";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

type WorkspaceBuildLogSectionProps = {
	status: ToolStatus;
	/** Build ID from the completed tool result. */
	buildId?: string;
};

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
export const WorkspaceBuildLogSection: React.FC<
	WorkspaceBuildLogSectionProps
> = ({ status, buildId }) => {
	const isRunning = status === "running";

	// Primary source: build ID from the chat binding, pushed via
	// pubsub when create_workspace or start_workspace persists it.
	const { workspaceId, buildId: chatBuildId } = useChatWorkspace();

	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const latestBuild = workspaceQuery.data?.latest_build;
	const activeBuildId =
		latestBuild && ACTIVE_BUILD_STATUSES.includes(latestBuild.status)
			? latestBuild.id
			: undefined;

	const effectiveBuildId = isRunning ? (chatBuildId ?? activeBuildId) : buildId;

	const watchedBuild =
		latestBuild?.id === effectiveBuildId ? latestBuild : undefined;
	const buildByIdQuery = useQuery({
		...workspaceBuildById(effectiveBuildId ?? ""),
		enabled:
			Boolean(effectiveBuildId) &&
			!isRunning &&
			(!workspaceId ||
				((workspaceQuery.isSuccess || workspaceQuery.isError) &&
					!watchedBuild)),
	});
	const build = watchedBuild ?? buildByIdQuery.data;

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

	const [prevTimeoutInputs, setPrevTimeoutInputs] = useState({
		effectiveBuildId,
		hasLogs,
	});
	if (
		prevTimeoutInputs.effectiveBuildId !== effectiveBuildId ||
		prevTimeoutInputs.hasLogs !== hasLogs
	) {
		setPrevTimeoutInputs({ effectiveBuildId, hasLogs });
		setTimedOut(false);
	}

	useEffect(() => {
		if (!effectiveBuildId || hasLogs) {
			return;
		}
		const timer = setTimeout(() => setTimedOut(true), LOG_LOAD_TIMEOUT_MS);
		return () => clearTimeout(timer);
	}, [effectiveBuildId, hasLogs]);

	const fetchFailed = !isRunning && completedLogsQuery.isError;

	if (!effectiveBuildId) {
		if (isRunning && workspaceId) {
			return <LogNotice icon="loading">Loading build logs…</LogNotice>;
		}
		return null;
	}

	let content: React.ReactNode;
	if (fetchFailed) {
		content = <LogNotice icon="warning">Failed to load build logs.</LogNotice>;
	} else if (timedOut && !hasLogs) {
		content = (
			<LogNotice icon="warning">
				Build logs are taking longer than expected.
			</LogNotice>
		);
	} else if (!isRunning && completedLogsQuery.isSuccess && !hasLogs) {
		content = <LogNotice>No build logs available.</LogNotice>;
	} else if (!logs || logs.length === 0) {
		content = <LogNotice icon="loading">Loading build logs…</LogNotice>;
	} else {
		content = (
			<ToolCall.Scrollable
				className="mt-1.5 rounded-md border text-2xs"
				viewportClassName="max-h-64"
				ariaLabel="Workspace build log"
				followEnd
			>
				{/* WorkspaceBuildLogs' scrollIntoView would also scroll the chat transcript. */}
				<WorkspaceBuildLogs
					logs={logs}
					sticky
					disableAutoscroll
					className="border-0 rounded-none"
				/>
			</ToolCall.Scrollable>
		);
	}

	return (
		<>
			{!build &&
				(isRunning ? workspaceQuery.isError : buildByIdQuery.isError) && (
					<LogNotice icon="warning">
						Unable to check whether build logs were truncated.
					</LogNotice>
				)}
			{build?.job.logs_overflowed && (
				<Alert severity="warning" className="mt-1.5">
					Provisioner logs exceeded the 1MB size limit. Later logs for this
					build were not saved.
				</Alert>
			)}
			{content}
		</>
	);
};
