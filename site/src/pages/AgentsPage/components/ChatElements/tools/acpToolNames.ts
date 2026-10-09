export const ACPToolNames = {
	SpawnAgent: "acp_spawn_agent",
	ListAgents: "acp_list_agents",
	WaitAgent: "acp_wait_agent",
	MessageAgent: "acp_message_agent",
	InterruptAgent: "acp_interrupt_agent",
} as const;

export type ACPToolName = (typeof ACPToolNames)[keyof typeof ACPToolNames];

const acpToolNames = new Set<string>(Object.values(ACPToolNames));

/** Narrows a tool name to one of the supported ACP tools. */
export const isACPToolName = (name: string): name is ACPToolName =>
	acpToolNames.has(name);
