import type {
	AgentFirewallLog,
	AIBridgeAgenticAction,
	AIBridgeThread,
} from "#/api/typesGenerated";

/**
 * Pure session-search helpers. Matching is case-insensitive substring
 * matching over the strict field scope from AIGOV-462: prompt text, tool
 * names, tool input JSON, and network call destinations.
 */

const normalizeQuery = (query: string): string => query.trim().toLowerCase();

const matchesToolCalls = (
	actions: readonly AIBridgeAgenticAction[],
	q: string,
) =>
	actions.some((action) =>
		action.tool_calls.some(
			(call) =>
				call.tool.toLowerCase().includes(q) ||
				call.input.toLowerCase().includes(q),
		),
	);

export const matchesThreadSearch = (
	thread: AIBridgeThread,
	query: string,
): boolean => {
	const q = normalizeQuery(query);
	if (q === "") {
		return true;
	}
	return (
		matchesToolCalls(thread.agentic_actions, q) ||
		(thread.prompt?.toLowerCase().includes(q) ?? false)
	);
};

export const matchesNetworkCallSearch = (
	call: AgentFirewallLog,
	query: string,
): boolean => {
	const q = normalizeQuery(query);
	if (q === "") {
		return true;
	}
	return call.detail.toLowerCase().includes(q);
};
