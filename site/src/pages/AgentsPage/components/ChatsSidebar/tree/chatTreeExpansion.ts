import type { ChatTreeModelNode } from "./chatTreeModel";

export const CHAT_TREE_EXPANSION_STORAGE_KEY = "agents.chat-tree-expanded";

/**
 * Persisted expansion state. Roots and organization nodes start expanded
 * and only record an explicit collapse; every other node starts collapsed
 * and only records an explicit expand. This keeps the defaults stable when
 * new organizations or roots appear after the state was first saved.
 */
export type ChatTreeExpansionState = {
	readonly expanded: ReadonlySet<string>;
	readonly collapsedTopLevel: ReadonlySet<string>;
	/** Nodes whose subagents are shown. */
	readonly subagents: ReadonlySet<string>;
};

export const EMPTY_CHAT_TREE_EXPANSION: ChatTreeExpansionState = {
	expanded: new Set(),
	collapsedTopLevel: new Set(),
	subagents: new Set(),
};

const isTopLevelKind = (node: ChatTreeModelNode) =>
	node.kind === "root" || node.kind === "organization";

export const isChatTreeNodeExpanded = (
	state: ChatTreeExpansionState,
	node: ChatTreeModelNode,
): boolean =>
	isTopLevelKind(node)
		? !state.collapsedTopLevel.has(node.id)
		: state.expanded.has(node.id);

const withMember = (
	set: ReadonlySet<string>,
	id: string,
	member: boolean,
): ReadonlySet<string> => {
	if (set.has(id) === member) {
		return set;
	}
	const next = new Set(set);
	if (member) {
		next.add(id);
	} else {
		next.delete(id);
	}
	return next;
};

export const setChatTreeNodeExpanded = (
	state: ChatTreeExpansionState,
	node: ChatTreeModelNode,
	expanded: boolean,
): ChatTreeExpansionState => {
	if (isTopLevelKind(node)) {
		const collapsedTopLevel = withMember(
			state.collapsedTopLevel,
			node.id,
			!expanded,
		);
		return collapsedTopLevel === state.collapsedTopLevel
			? state
			: { ...state, collapsedTopLevel };
	}
	const expandedSet = withMember(state.expanded, node.id, expanded);
	return expandedSet === state.expanded
		? state
		: { ...state, expanded: expandedSet };
};

export const setChatTreeSubagentsShown = (
	state: ChatTreeExpansionState,
	id: string,
	shown: boolean,
): ChatTreeExpansionState => {
	const subagents = withMember(state.subagents, id, shown);
	return subagents === state.subagents ? state : { ...state, subagents };
};

const readStringArray = (value: unknown): string[] =>
	Array.isArray(value)
		? value.filter((item): item is string => typeof item === "string")
		: [];

export const loadChatTreeExpansion = (): ChatTreeExpansionState => {
	let stored: string | null;
	try {
		stored = localStorage.getItem(CHAT_TREE_EXPANSION_STORAGE_KEY);
	} catch {
		return EMPTY_CHAT_TREE_EXPANSION;
	}
	if (!stored) {
		return EMPTY_CHAT_TREE_EXPANSION;
	}
	let parsed: unknown;
	try {
		parsed = JSON.parse(stored);
	} catch {
		return EMPTY_CHAT_TREE_EXPANSION;
	}
	if (!parsed || typeof parsed !== "object") {
		return EMPTY_CHAT_TREE_EXPANSION;
	}
	const record = parsed as Record<string, unknown>;
	return {
		expanded: new Set(readStringArray(record.expanded)),
		collapsedTopLevel: new Set(readStringArray(record.collapsedTopLevel)),
		subagents: new Set(readStringArray(record.subagents)),
	};
};

export const persistChatTreeExpansion = (
	state: ChatTreeExpansionState,
): void => {
	try {
		localStorage.setItem(
			CHAT_TREE_EXPANSION_STORAGE_KEY,
			JSON.stringify({
				expanded: [...state.expanded],
				collapsedTopLevel: [...state.collapsedTopLevel],
				subagents: [...state.subagents],
			}),
		);
	} catch {
		// Storage failures only lose persistence; the tree still works.
	}
};
