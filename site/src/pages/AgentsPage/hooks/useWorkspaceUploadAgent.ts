import { useQuery } from "react-query";
import { chatWorkspaceAgent } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { findWorkspaceAgent } from "#/utils/workspace";

/**
 * Resolves the agent that workspace uploads target: the chat's bound
 * agent when it is in the workspace's latest build, otherwise the agent
 * the server selects for the workspace. `isResolved` stays false while
 * that selection is loading or failed to load, so callers can tell an
 * unknown target apart from a workspace without an eligible agent.
 * `canUpload` requires a running latest build and a connected target.
 */
export const useWorkspaceUploadAgent = (
	workspace: TypesGen.Workspace | undefined,
	boundAgentId?: string,
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

	const selectedAgentId = selectionQuery.data?.agent_id;
	let agent = boundAgent;
	if (needsSelection && workspace && selectedAgentId) {
		agent = findWorkspaceAgent(workspace, selectedAgentId);
	}
	const isResolved = !needsSelection || selectionQuery.data !== undefined;
	return {
		isResolved,
		canUpload: isResolved && isRunning && agent?.status === "connected",
	};
};
