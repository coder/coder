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
	toolMatch: boolean;
}

/**
 * Reports which search axis matched, in one walk. The filter, the
 * auto-expand signal, and the prompt window all derive from this.
 * An empty query matches the prompt axis but reports no tool match.
 */
export const classifyThreadSearch = (
	thread: AIBridgeThread,
	query: string,
): ThreadSearchClassification => {
	const q = normalizeQuery(query);
	if (q === "") {
		return { promptMatch: true, toolMatch: false };
	}
	return {
		promptMatch: thread.prompt?.toLowerCase().includes(q) ?? false,
		toolMatch: thread.agentic_actions.some((action) =>
			action.tool_calls.some(
				(call) =>
					call.tool.toLowerCase().includes(q) ||
					call.input.toLowerCase().includes(q),
			),
		),
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
		if (thread.prompt) {
			count += countOccurrences(thread.prompt, q);
		}
		for (const action of thread.agentic_actions) {
			for (const call of action.tool_calls) {
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

/**
 * Returns the [start, end) indices of a window centered on the first match,
 * or null when there is no match or the text fits inside the window.
 */
export const windowAroundFirstMatch = (
	text: string,
	query: string,
	windowChars: number,
): { start: number; end: number } | null => {
	const q = normalizeQuery(query);
	if (q === "" || text.length <= windowChars) {
		return null;
	}
	const first = text.toLowerCase().indexOf(q);
	if (first === -1) {
		return null;
	}
	const center = first + q.length / 2;
	let start = Math.round(center - windowChars / 2);
	start = Math.max(0, Math.min(start, text.length - windowChars));
	return { start, end: start + windowChars };
};
