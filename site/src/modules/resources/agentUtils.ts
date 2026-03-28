import type { WorkspaceAgent, WorkspaceResource } from "#/api/typesGenerated";

// Chat agents use this suffix for AI chat routing and stay hidden in the UI.
export const CHAT_AGENT_SUFFIX = "-coderd-chat";

export const isChatAgent = (agent: Pick<WorkspaceAgent, "name">): boolean => {
	return agent.name.toLowerCase().endsWith(CHAT_AGENT_SUFFIX);
};

export const getVisibleWorkspaceAgents = (
	agents: readonly WorkspaceAgent[] | undefined,
): readonly WorkspaceAgent[] => {
	if (!agents) {
		return [];
	}

	const agentsById = new Map(agents.map((agent) => [agent.id, agent]));
	const hiddenAgentIds = new Set<string>();

	const shouldHideAgent = (agent: WorkspaceAgent): boolean => {
		if (hiddenAgentIds.has(agent.id)) {
			return true;
		}

		let currentAgent: WorkspaceAgent | undefined = agent;
		const visitedAgentIds = new Set<string>();
		while (currentAgent && !visitedAgentIds.has(currentAgent.id)) {
			visitedAgentIds.add(currentAgent.id);
			if (isChatAgent(currentAgent)) {
				hiddenAgentIds.add(agent.id);
				return true;
			}
			currentAgent = currentAgent.parent_id
				? agentsById.get(currentAgent.parent_id)
				: undefined;
		}

		return false;
	};

	return agents.filter((agent) => !shouldHideAgent(agent));
};

export const countVisibleWorkspaceAgents = (
	resource: Pick<WorkspaceResource, "agents">,
): number => {
	return getVisibleWorkspaceAgents(resource.agents).length;
};
