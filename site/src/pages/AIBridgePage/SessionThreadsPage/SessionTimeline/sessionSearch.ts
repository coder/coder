import type { AgentFirewallLog, AIBridgeThread } from "#/api/typesGenerated";

/**
 * Pure session-search helpers. Matching is case-insensitive substring
 * matching over the strict field scope: prompt text, tool names, tool
 * input JSON, and network call detail (URL for HTTP, host for DNS).
 */

const normalizeQuery = (query: string): string => query.trim().toLowerCase();

const matchesToolSearch = (thread: AIBridgeThread, q: string): boolean =>
	thread.agentic_actions.some((action) =>
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
		matchesToolSearch(thread, q) ||
		(thread.prompt?.toLowerCase().includes(q) ?? false)
	);
};

/**
 * Returns true when the query matches a tool name or tool input in the
 * thread's agentic loop. Used to surface why a thread matched when the
 * match lives inside a collapsed section.
 */
export const matchesThreadToolQuery = (
	thread: AIBridgeThread,
	query: string,
): boolean => {
	const q = normalizeQuery(query);
	if (q === "") {
		return false;
	}
	return matchesToolSearch(thread, q);
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
