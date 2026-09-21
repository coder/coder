import type * as TypesGen from "#/api/typesGenerated";

export type WorkspaceFailure = {
	kind: "build" | "agent";
	buildId: string;
	error: string;
};

const failedAgentLifecycleStates: ReadonlySet<TypesGen.WorkspaceAgentLifecycle> =
	new Set(["start_error", "start_timeout"]);

/**
 * Derives whether a workspace is in a state the AI debugging panel should
 * help with: the latest build failed, or the build succeeded but an agent
 * never became usable (startup script error or timeout, or the agent never
 * connected). Returns undefined when the workspace is healthy or still
 * transitioning.
 */
export const getWorkspaceFailure = (
	workspace: TypesGen.Workspace,
): WorkspaceFailure | undefined => {
	const build = workspace.latest_build;
	if (build.status === "failed") {
		return {
			kind: "build",
			buildId: build.id,
			error:
				build.job.error?.trim() ||
				`${build.transition} build finished with status ${build.job.status}`,
		};
	}
	if (build.status !== "running") {
		return undefined;
	}
	for (const resource of build.resources) {
		for (const agent of resource.agents ?? []) {
			if (failedAgentLifecycleStates.has(agent.lifecycle_state)) {
				return {
					kind: "agent",
					buildId: build.id,
					error:
						agent.lifecycle_state === "start_timeout"
							? `agent "${agent.name}" startup script timed out`
							: `agent "${agent.name}" startup script failed`,
				};
			}
			if (agent.status === "timeout") {
				return {
					kind: "agent",
					buildId: build.id,
					error: `agent "${agent.name}" never connected`,
				};
			}
		}
	}
	return undefined;
};
