import { useQuery } from "react-query";
import { workspaceById } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import { useChatWorkspace } from "../../../context/ChatWorkspaceContext";
import { getWorkspaceAgent } from "../../ChatConversation/chatHelpers";

export type WorkspaceToolAction = "create" | "start" | "stop";

type WorkspaceToolStageInput = {
	action: WorkspaceToolAction;
	workspace: Workspace | undefined;
	/** Build ID from the chat binding. */
	callBuildId: string | undefined;
	chatAgentId: string | undefined;
};

/**
 * Label for what a running workspace tool call is waiting on, derived
 * from the watched workspace. Returns undefined when the latest build
 * is not the call's build, so the caller shows its generic label.
 *
 * The stages follow the backend waits in `waitForBuild` and
 * `waitForAgentReady` (coderd/x/chatd/chattool/createworkspace.go).
 */
export const getWorkspaceToolStage = ({
	action,
	workspace,
	callBuildId,
	chatAgentId,
}: WorkspaceToolStageInput): string | undefined => {
	if (!workspace || !callBuildId) {
		return undefined;
	}
	const build = workspace.latest_build;
	if (build.id !== callBuildId) {
		return undefined;
	}
	const isStop = action === "stop";
	if (build.transition !== (isStop ? "stop" : "start")) {
		return undefined;
	}
	if (build.status === "pending") {
		return "Waiting in build queue…";
	}
	if (isStop) {
		return build.status === "stopping" ? "Stopping workspace…" : undefined;
	}
	if (build.status === "starting") {
		return "Building workspace…";
	}
	if (build.status !== "running") {
		return undefined;
	}
	const agent = getWorkspaceAgent(workspace, chatAgentId);
	if (agent?.status !== "connected") {
		return "Waiting for workspace agent to connect…";
	}
	if (
		agent.lifecycle_state === "created" ||
		agent.lifecycle_state === "starting"
	) {
		return "Running startup scripts…";
	}
	return undefined;
};

/**
 * Running stage label for the chat's workspace tool row, or undefined
 * when the row should show its generic running label. The workspace
 * query has no refetch interval because useWorkspaceWatch keeps this
 * cache entry current for the chat.
 */
export const useWorkspaceToolStage = (
	action: WorkspaceToolAction,
	isRunning: boolean,
): string | undefined => {
	const { workspaceId, buildId, agentId } = useChatWorkspace();
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: isRunning && Boolean(workspaceId),
	});
	if (!isRunning) {
		return undefined;
	}
	return getWorkspaceToolStage({
		action,
		workspace: workspaceQuery.data,
		callBuildId: buildId,
		chatAgentId: agentId,
	});
};
