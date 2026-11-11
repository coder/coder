import {
	WorkloadShapeEnv,
	type WorkspaceAgent,
	type WorkspaceResource,
} from "#/api/typesGenerated";

/**
 * A workload shape is a template-declared agent that never runs. It only
 * defines what the daemons started by `coder run` look like.
 */
const isWorkloadShape = (agent: WorkspaceAgent): boolean => {
	// The API redacts environment values, and sends null instead of an empty
	// object, so only the presence of the marker can be tested. The daemons
	// cloned from a shape inherit the marker, hence the parent check.
	return (
		agent.parent_id === null &&
		agent.environment_variables?.[WorkloadShapeEnv] !== undefined
	);
};

export const hasWorkloadShape = (
	resources: readonly WorkspaceResource[],
): boolean => {
	return resources.some((resource) => resource.agents?.some(isWorkloadShape));
};

/**
 * Removes workload shapes and the daemons cloned from them. Shapes never
 * connect, and their daemons are listed as workloads instead of as agents.
 */
export const withoutWorkloadAgents = (
	resources: readonly WorkspaceResource[],
): WorkspaceResource[] => {
	const shapeIds = new Set(
		resources
			.flatMap((resource) => resource.agents ?? [])
			.filter(isWorkloadShape)
			.map((agent) => agent.id),
	);
	if (shapeIds.size === 0) {
		return [...resources];
	}
	return resources.map((resource) => ({
		...resource,
		agents: resource.agents?.filter(
			(agent) =>
				!shapeIds.has(agent.id) &&
				(agent.parent_id === null || !shapeIds.has(agent.parent_id)),
		),
	}));
};
