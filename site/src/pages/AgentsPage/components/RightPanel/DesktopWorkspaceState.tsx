import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { API, ParameterValidationError } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import { startWorkspace } from "#/api/queries/workspaces";
import type {
	Workspace,
	WorkspaceAgentStatus,
	WorkspaceStatus,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";

/** Build statuses in which the workspace is not running and not in flight. */
const stoppedWorkspaceStatuses: readonly WorkspaceStatus[] = [
	"stopped",
	"failed",
	"canceled",
];

/**
 * Whether the desktop can be dialed. The desktop endpoint rejects any
 * agent that is not connected, so this is the precondition for opening
 * the VNC session.
 */
export const isDesktopReachable = (
	workspaceStatus: WorkspaceStatus,
	agentStatus: WorkspaceAgentStatus | undefined,
): boolean => workspaceStatus === "running" && agentStatus === "connected";

/**
 * The build that brings a stopped workspace back up, mirroring the
 * workspace page's actions:
 * - `update` when the template requires its active version and the
 *   workspace is outdated, since a plain start of the previous version
 *   is rejected by the backend.
 * - `retry` after a failed start, which stops first for a clean slate.
 * - `start` otherwise.
 * `undefined` when the workspace is not stopped, or when its last build
 * was a failed stop or delete, which must be retried from the workspace
 * page rather than started over.
 */
export const getDesktopStartAction = (
	workspace: Workspace,
): "start" | "retry" | "update" | undefined => {
	const { status, transition } = workspace.latest_build;
	if (!stoppedWorkspaceStatuses.includes(status)) {
		return undefined;
	}
	if (workspace.outdated && workspace.template_require_active_version) {
		return "update";
	}
	if (status === "failed") {
		return transition === "start" ? "retry" : undefined;
	}
	return "start";
};

export type DesktopWorkspaceStateProps = {
	workspace: Workspace;
	onStartWorkspace: () => void;
	isStartingWorkspace: boolean;
};

/**
 * Renders why the desktop cannot connect yet at the workspace level:
 * stopped (with a start action), deleted, or still transitioning.
 * Callers gate on `isDesktopReachable` and fall through to the
 * connection status once it returns true.
 */
export const DesktopWorkspaceState: FC<DesktopWorkspaceStateProps> = ({
	workspace,
	onStartWorkspace,
	isStartingWorkspace,
}) => {
	const workspaceStatus = workspace.latest_build.status;
	const startAction = getDesktopStartAction(workspace);

	if (startAction) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-3 text-content-secondary">
				<span className="text-center text-sm">
					{startAction === "update"
						? "The workspace is stopped and must be updated to the template's active version to start."
						: "The workspace is stopped. Start it to reconnect to the desktop."}
				</span>
				<Button
					variant="outline"
					size="sm"
					onClick={onStartWorkspace}
					disabled={isStartingWorkspace}
				>
					<Spinner loading={isStartingWorkspace} />
					{startAction === "update" ? "Update and start" : "Start workspace"}
				</Button>
			</div>
		);
	}

	if (stoppedWorkspaceStatuses.includes(workspaceStatus)) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<span className="text-center text-sm">
					{`The last workspace build failed to ${workspace.latest_build.transition}.`}
				</span>
				<Link
					href={`/@${workspace.owner_name}/${workspace.name}`}
					target="_blank"
					rel="noreferrer"
				>
					Retry it from the workspace page
				</Link>
			</div>
		);
	}

	if (workspaceStatus === "deleted") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<span className="text-sm">The workspace has been deleted.</span>
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
			<Spinner loading className="size-6" />
			<span className="text-sm">
				{workspaceStatus === "running"
					? "Waiting for the workspace agent to connect..."
					: `Workspace is ${workspaceStatus}...`}
			</span>
		</div>
	);
};

/**
 * Start mutation for the desktop's workspace with error reporting. The
 * build is chosen by `getDesktopStartAction`. The workspace may still be
 * loading in the pop-out window, so the start action rejects until it
 * arrives.
 */
export const useStartDesktopWorkspace = (workspace: Workspace | undefined) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const start = workspace ? startWorkspace(workspace, queryClient) : undefined;
	const { mutate, isPending } = useMutation({
		mutationFn: () => {
			if (!workspace) {
				return Promise.reject(new Error("Workspace is not loaded."));
			}
			const versionId = workspace.latest_build.template_version_id;
			switch (getDesktopStartAction(workspace)) {
				case "update":
					return API.updateWorkspace(workspace);
				case "retry":
					return API.retryWorkspace(workspace, versionId);
				case "start":
					return API.startWorkspace(workspace.id, versionId);
				default:
					return Promise.reject(
						new Error("The workspace cannot be started from the desktop."),
					);
			}
		},
		// Every action ends in a start build, so the cache update is shared.
		onSuccess: (build) => start?.onSuccess(build),
		onError: (error) => {
			if (workspace && error instanceof ParameterValidationError) {
				toast.error(
					"The active template version has parameters that must be set before the workspace can be updated.",
					{
						action: {
							label: "Set parameters",
							onClick: () =>
								navigate(
									`/@${workspace.owner_name}/${workspace.name}/settings/parameters?templateVersionId=${error.versionId}`,
								),
						},
					},
				);
				return;
			}
			toast.error(getErrorMessage(error, "Failed to start workspace."));
		},
	});
	return { startWorkspace: () => mutate(), isStartingWorkspace: isPending };
};
