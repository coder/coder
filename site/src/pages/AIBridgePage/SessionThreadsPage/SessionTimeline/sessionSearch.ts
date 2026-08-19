import type { AgentFirewallLog, AIBridgeThread } from "#/api/typesGenerated";

/**
 * Pure session-search helpers. Matching is case-insensitive substring
 * matching over the strict field scope: prompt text, tool names, tool
 * input JSON, and network call detail (URL for HTTP).
 */

const normalizeQuery = (query: string): string => query.trim().toLowerCase();

// Counts non-overlapping, case-insensitive occurrences of the query.
const countOccurrences = (text: string, query: string): number => {
	const q = normalizeQuery(query);
	if (q === "") {
		return 0;
	}
	const haystack = text.toLowerCase();
	let count = 0;
	let index = haystack.indexOf(q);
	while (index !== -1) {
		count += 1;
		index = haystack.indexOf(q, index + q.length);
	}
	return count;
};

interface ThreadSearchClassification {
	promptMatch: boolean;
	/** IDs of tool calls whose tool name or input matched. */
	toolCallIds: Set<string>;
}

/**
 * Reports which search axis matched and which tool calls matched, in one
 * walk. The filter, the auto-expand signal, the prompt window, and the
 * tool-call filter all derive from this. An empty query matches the prompt
 * axis but reports no tool matches.
 */
export const classifyThreadSearch = (
	thread: AIBridgeThread,
	query: string,
): ThreadSearchClassification => {
	const q = normalizeQuery(query);
	const toolCallIds = new Set<string>();
	if (q !== "") {
		for (const action of thread.agentic_actions) {
			for (const call of action.tool_calls) {
				if (
					call.tool.toLowerCase().includes(q) ||
					call.input.toLowerCase().includes(q)
				) {
					toolCallIds.add(call.id);
				}
			}
		}
	}
	return {
		promptMatch:
			q === "" ? true : (thread.prompt?.toLowerCase().includes(q) ?? false),
		toolCallIds,
	};
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

/**
 * Counts how many times the query occurs across the session's searchable
 * fields: thread prompts, tool names, tool input JSON, and network call
 * detail. This is the "N results" figure shown under the search input.
 */
export const countSessionSearchResults = (
	threads: readonly AIBridgeThread[],
	networkCalls: readonly AgentFirewallLog[],
	query: string,
): number => {
	const q = normalizeQuery(query);
	if (q === "") {
		return 0;
	}
	let count = 0;
	for (const thread of threads) {
		const { toolCallIds } = classifyThreadSearch(thread, q);
		if (thread.prompt) {
			count += countOccurrences(thread.prompt, q);
		}
		for (const action of thread.agentic_actions) {
			for (const call of action.tool_calls) {
				if (!toolCallIds.has(call.id)) {
					continue;
				}
				count += countOccurrences(call.tool, q);
				count += countOccurrences(call.input, q);
			}
		}
	}
	for (const call of networkCalls) {
		count += countOccurrences(call.detail, q);
	}
	return count;
};

interface MatchSegment {
	text: string;
	match: boolean;
}

// Splits the text into match and non-match segments for bold rendering.
export const splitMatchSegments = (
	text: string,
	query: string,
): MatchSegment[] => {
	const q = normalizeQuery(query);
	if (q === "") {
		return [{ text, match: false }];
	}
	const haystack = text.toLowerCase();
	const segments: MatchSegment[] = [];
	let cursor = 0;
	let index = haystack.indexOf(q);
	while (index !== -1) {
		if (index > cursor) {
			segments.push({ text: text.slice(cursor, index), match: false });
		}
		segments.push({
			text: text.slice(index, index + q.length),
			match: true,
		});
		cursor = index + q.length;
		index = haystack.indexOf(q, cursor);
	}
	if (cursor < text.length) {
		segments.push({ text: text.slice(cursor), match: false });
	}
	return segments;
};
