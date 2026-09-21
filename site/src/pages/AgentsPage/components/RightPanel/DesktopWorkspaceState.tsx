import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { startWorkspace } from "#/api/queries/workspaces";
import type {
	Workspace,
	WorkspaceAgentStatus,
	WorkspaceStatus,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";

/** Build statuses from which the workspace can be started again. */
const startableWorkspaceStatuses: readonly WorkspaceStatus[] = [
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

export interface DesktopWorkspaceStateProps {
	workspaceStatus: WorkspaceStatus;
	onStartWorkspace: () => void;
	isStartingWorkspace: boolean;
}

/**
 * Renders why the desktop cannot connect yet at the workspace level:
 * stopped (with a start action), deleted, or still transitioning.
 * Callers gate on `isDesktopReachable` and fall through to the
 * connection status once it returns true.
 */
export const DesktopWorkspaceState: FC<DesktopWorkspaceStateProps> = ({
	workspaceStatus,
	onStartWorkspace,
	isStartingWorkspace,
}) => {
	if (startableWorkspaceStatuses.includes(workspaceStatus)) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-3 text-content-secondary">
				<span className="text-center text-sm">
					The workspace is stopped. Start it to reconnect to the desktop.
				</span>
				<Button
					variant="outline"
					size="sm"
					onClick={onStartWorkspace}
					disabled={isStartingWorkspace}
				>
					<Spinner loading={isStartingWorkspace} />
					Start workspace
				</Button>
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
 * workspace may still be loading in the pop-out window, so the start
 * action rejects until it arrives.
 */
export const useStartDesktopWorkspace = (workspace: Workspace | undefined) => {
	const queryClient = useQueryClient();
	const start = workspace ? startWorkspace(workspace, queryClient) : undefined;
	const { mutate, isPending } = useMutation({
		mutationFn: () =>
			start
				? start.mutationFn({})
				: Promise.reject(new Error("Workspace is not loaded.")),
		onSuccess: (build) => start?.onSuccess(build),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to start workspace."));
		},
	});
	return { startWorkspace: () => mutate(), isStartingWorkspace: isPending };
};
