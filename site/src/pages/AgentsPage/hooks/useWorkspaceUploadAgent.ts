import { useQuery } from "react-query";
import { chatWorkspaceAgent } from "#/api/queries/chats";
import { workspaceById } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { findWorkspaceAgent } from "#/utils/workspace";

export const workspaceUploadAgentLookupFailedMessage =
	"Couldn't determine which workspace agent receives uploads. Try again in a moment.";

export const workspaceUploadNoEligibleAgentMessage =
	"This file type is uploaded into the chat's workspace, which has no agent that can receive it. Check the workspace's agent configuration.";

const useUploadTarget = (
	workspace: TypesGen.Workspace | undefined,
	boundAgentId: string | undefined,
) => {
	const boundAgent =
		workspace && boundAgentId
			? findWorkspaceAgent(workspace, boundAgentId)
			: undefined;
	const isRunning = workspace?.latest_build.status === "running";
	const needsSelection =
		workspace !== undefined && isRunning && boundAgent === undefined;
	const selectionQuery = useQuery({
		...chatWorkspaceAgent(
			workspace?.id ?? "",
			workspace?.latest_build.id ?? "",
		),
		enabled: needsSelection,
	});

	const selection = needsSelection ? selectionQuery.data : undefined;
	const selectedAgentId = selection?.agent_id;
	const agent =
		workspace && selectedAgentId
			? findWorkspaceAgent(workspace, selectedAgentId)
			: boundAgent;
	const isResolved = !needsSelection || selection !== undefined;
	return {
		isResolved,
		lookupFailed: needsSelection && selectionQuery.isError,
		isSelectionMissing: selection !== undefined && agent === undefined,
		noEligibleAgent: selection !== undefined && selectedAgentId === undefined,
		canUpload: isResolved && isRunning && agent?.status === "connected",
	};
};

/**
 * Resolves the agent that workspace uploads target: the chat's bound
 * agent when it is in the workspace's latest build, otherwise the agent
 * the server selects for the workspace. `isResolved` stays false while
 * that selection is loading or failed to load, so callers can tell an
 * unknown target apart from a workspace without an eligible agent;
 * `lookupFailed` marks the failed case until a retry succeeds, and
 * `noEligibleAgent` a running workspace where the server selects none.
 */
export const useWorkspaceUploadAgent = (
	workspace: TypesGen.Workspace | undefined,
	boundAgentId?: string,
) => {
	const target = useUploadTarget(workspace, boundAgentId);
	// The server selects from its current latest build, which the given
	// workspace can predate, since the new-chat page does not refetch its
	// list while open. A selection missing from the given workspace is
	// resolved against a fresh copy; a cached copy older than it is ignored.
	const needsFreshCopy = target.isSelectionMissing;
	const freshQuery = useQuery({
		...workspaceById(workspace?.id ?? ""),
		enabled: needsFreshCopy,
		refetchInterval: ({ state }) => (state.status === "error" ? 5_000 : false),
	});
	const freshWorkspace =
		needsFreshCopy &&
		workspace &&
		freshQuery.data &&
		freshQuery.data.latest_build.build_number >=
			workspace.latest_build.build_number
			? freshQuery.data
			: undefined;
	const freshTarget = useUploadTarget(freshWorkspace, boundAgentId);
	// A newer build's agent is often still connecting, so the fresh copy
	// is read again until that agent can take uploads or the server
	// selects none.
	useQuery({
		...workspaceById(workspace?.id ?? ""),
		enabled:
			freshWorkspace !== undefined &&
			freshWorkspace.latest_build.id !== workspace?.latest_build.id &&
			!freshTarget.canUpload &&
			!freshTarget.noEligibleAgent,
		refetchInterval: 5_000,
	});

	if (!needsFreshCopy) {
		return {
			isResolved: target.isResolved,
			lookupFailed: target.lookupFailed,
			noEligibleAgent: false,
			canUpload: target.canUpload,
		};
	}
	return {
		isResolved: freshWorkspace !== undefined && freshTarget.isResolved,
		lookupFailed: freshQuery.isError || freshTarget.lookupFailed,
		noEligibleAgent: freshTarget.noEligibleAgent,
		canUpload: freshTarget.canUpload,
	};
};
