import type { AgentFirewallLog, AIBridgeThread } from "#/api/typesGenerated";

/**
 * Pure session-search helpers. Matching is case-insensitive substring
 * matching over the strict field scope: prompt text, tool names, tool
 * input JSON, and network call detail (URL for HTTP).
 */

const normalizeQuery = (query: string): string => query.trim().toLowerCase();

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
 * Counts the events that match the query: each matching thread plus each
 * matching network call. This is the "N matches" figure shown under the
 * search input. It reuses the same classifiers as the filter so the two can
 * never disagree on what counts as a match.
 */
export const countSessionSearchMatches = (
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
		const { promptMatch, toolCallIds } = classifyThreadSearch(thread, q);
		if (promptMatch || toolCallIds.size > 0) {
			count += 1;
		}
	}
	for (const call of networkCalls) {
		if (matchesNetworkCallSearch(call, q)) {
			count += 1;
		}
	}
	return count;
};

interface MatchSegment {
	text: string;
	match: boolean;
}

// Splits the text into match and non-match segments for bold rendering.
// Matches against the original text one window at a time rather than a
// lowercased copy, because lowercasing can change length (e.g. Turkish
// dotted capital I) and misalign the slice indices.
export const splitMatchSegments = (
	text: string,
	query: string,
): MatchSegment[] => {
	const q = normalizeQuery(query);
	if (q === "") {
		return [{ text, match: false }];
	}
	const segments: MatchSegment[] = [];
	let cursor = 0;
	let i = 0;
	while (i + q.length <= text.length) {
		if (text.slice(i, i + q.length).toLowerCase() === q) {
			if (i > cursor) {
				segments.push({ text: text.slice(cursor, i), match: false });
			}
			segments.push({ text: text.slice(i, i + q.length), match: true });
			i += q.length;
			cursor = i;
		} else {
			i++;
		}
	}
	if (cursor < text.length) {
		segments.push({ text: text.slice(cursor), match: false });
	}
	return segments;
};
