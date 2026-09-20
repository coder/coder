import type { Chat, ChatTreeResponse } from "#/api/typesGenerated";
import { ChatTreeMaxDepth } from "#/api/typesGenerated";
import { asNonEmptyString } from "../../ChatConversation/blockUtils";

export type ChatTreeNodeKind = "organization" | "root" | "chat" | "subagent";

export type ChatTreeModelNode = {
	readonly id: string;
	readonly kind: ChatTreeNodeKind;
	/** Absent on synthetic organization nodes. */
	readonly chat?: Chat;
	readonly organizationId: string;
	readonly label: string;
	/** 1-based, derived from the client hierarchy, used as aria-level. */
	readonly level: number;
	readonly parentId?: string;
};

export type ChatTreeModel = {
	readonly topLevelIds: readonly string[];
	readonly nodesById: ReadonlyMap<string, ChatTreeModelNode>;
	readonly childrenById: ReadonlyMap<string, readonly string[]>;
};

export type ChatTreeSource = {
	readonly organization: { readonly id: string; readonly displayName: string };
	readonly response: ChatTreeResponse;
	/** Subagents fetched per node, keyed by the parent chat id. */
	readonly subagentsByParent?: ReadonlyMap<string, readonly Chat[]>;
};

export const organizationTreeNodeId = (organizationId: string) =>
	`organization:${organizationId}`;

const EMPTY_CHILDREN: readonly string[] = [];

/**
 * Builds the sidebar hierarchy from one tree response per organization.
 * Named children hang off parent_chat_id; a row whose parent is absent
 * from the response (the archived view omits active ancestors) attaches
 * under the organization root, or at the top level when the response has
 * no root. Levels are always recomputed here because server depth counts
 * omitted ancestors. With several organizations each one gets a synthetic
 * organization node above its root.
 */
export const buildChatTreeModel = (
	sources: readonly ChatTreeSource[],
): ChatTreeModel => {
	const nodesById = new Map<string, ChatTreeModelNode>();
	const childrenById = new Map<string, string[]>();
	const topLevelIds: string[] = [];
	const useOrganizationNodes = sources.length > 1;

	for (const { organization, response, subagentsByParent } of sources) {
		const rows = response.chats.filter((chat) => chat.kind !== "subagent");
		const rowsById = new Map(rows.map((chat) => [chat.id, chat]));
		const rootId = response.root_chat_id ?? undefined;
		const rootRow = rootId ? rowsById.get(rootId) : undefined;

		const organizationTopLevel: string[] = [];
		let baseLevel = 1;
		if (useOrganizationNodes) {
			const organizationNodeId = organizationTreeNodeId(organization.id);
			nodesById.set(organizationNodeId, {
				id: organizationNodeId,
				kind: "organization",
				organizationId: organization.id,
				label: organization.displayName,
				level: 1,
			});
			childrenById.set(organizationNodeId, organizationTopLevel);
			topLevelIds.push(organizationNodeId);
			baseLevel = 2;
		}
		const organizationParentId = useOrganizationNodes
			? organizationTreeNodeId(organization.id)
			: undefined;

		// Resolve each row's parent before assigning levels so that
		// attachment does not depend on response order.
		const parentOf = new Map<string, string | undefined>();
		for (const chat of rows) {
			if (chat.id === rootRow?.id) {
				parentOf.set(chat.id, undefined);
				continue;
			}
			const declaredParent = asNonEmptyString(chat.parent_chat_id);
			const parentPresent =
				declaredParent !== undefined &&
				declaredParent !== chat.id &&
				rowsById.has(declaredParent);
			parentOf.set(chat.id, parentPresent ? declaredParent : rootRow?.id);
		}

		const localChildren = new Map<string, string[]>();
		const localTopLevel: string[] = [];
		for (const chat of rows) {
			const parentId = parentOf.get(chat.id);
			if (parentId === undefined) {
				localTopLevel.push(chat.id);
				continue;
			}
			const siblings = localChildren.get(parentId) ?? [];
			siblings.push(chat.id);
			localChildren.set(parentId, siblings);
		}

		// Depth-first walk from the top assigns levels; rows unreachable
		// through parent links (a parent cycle in stale data) fall back to
		// the top level so nothing silently disappears.
		const visited = new Set<string>();
		const placeRow = (
			chat: Chat,
			level: number,
			parentId: string | undefined,
			siblingList: string[],
		) => {
			if (visited.has(chat.id)) {
				return;
			}
			visited.add(chat.id);
			siblingList.push(chat.id);
			nodesById.set(chat.id, {
				id: chat.id,
				kind: chat.kind === "root" ? "root" : "chat",
				chat,
				organizationId: organization.id,
				label: chat.title,
				level,
				parentId,
			});
			const children: string[] = [];
			childrenById.set(chat.id, children);
			for (const childId of localChildren.get(chat.id) ?? []) {
				const child = rowsById.get(childId);
				if (child) {
					placeRow(child, level + 1, chat.id, children);
				}
			}
			for (const subagent of subagentsByParent?.get(chat.id) ?? []) {
				if (visited.has(subagent.id)) {
					continue;
				}
				visited.add(subagent.id);
				children.push(subagent.id);
				nodesById.set(subagent.id, {
					id: subagent.id,
					kind: "subagent",
					chat: subagent,
					organizationId: organization.id,
					label: subagent.title,
					level: level + 1,
					parentId: chat.id,
				});
				childrenById.set(subagent.id, []);
			}
		};

		const targetTopLevel = useOrganizationNodes
			? organizationTopLevel
			: topLevelIds;
		for (const id of localTopLevel) {
			const chat = rowsById.get(id);
			if (chat) {
				placeRow(chat, baseLevel, organizationParentId, targetTopLevel);
			}
		}
		for (const chat of rows) {
			if (!visited.has(chat.id)) {
				placeRow(chat, baseLevel, organizationParentId, targetTopLevel);
			}
		}
	}

	return { topLevelIds, nodesById, childrenById };
};

export const getChatTreeChildren = (
	model: ChatTreeModel,
	id: string,
): readonly string[] => model.childrenById.get(id) ?? EMPTY_CHILDREN;

export type ChatTreeVisibleRow = {
	readonly id: string;
	readonly node: ChatTreeModelNode;
	readonly hasChildren: boolean;
	readonly isExpanded: boolean;
	readonly setSize: number;
	readonly posInSet: number;
};

/**
 * Orders the rows a user can currently see. The same list drives
 * rendering and keyboard navigation so both agree on what "visible" means.
 * When `visible` is given, nodes outside it and their subtrees are
 * skipped and set sizes count only the remaining siblings.
 */
export const flattenVisibleTree = (
	model: ChatTreeModel,
	options: {
		readonly isExpanded: (node: ChatTreeModelNode) => boolean;
		readonly visible?: ReadonlySet<string>;
	},
): ChatTreeVisibleRow[] => {
	const rows: ChatTreeVisibleRow[] = [];
	const walk = (ids: readonly string[]) => {
		const shown = ids.filter(
			(id) =>
				model.nodesById.has(id) &&
				(options.visible === undefined || options.visible.has(id)),
		);
		for (const [index, id] of shown.entries()) {
			const node = model.nodesById.get(id);
			if (!node) {
				continue;
			}
			const children = getChatTreeChildren(model, id).filter(
				(childId) =>
					options.visible === undefined || options.visible.has(childId),
			);
			const hasChildren = children.length > 0;
			const isExpanded = hasChildren && options.isExpanded(node);
			rows.push({
				id,
				node,
				hasChildren,
				isExpanded,
				setSize: shown.length,
				posInSet: index + 1,
			});
			if (isExpanded) {
				walk(children);
			}
		}
	};
	walk(model.topLevelIds);
	return rows;
};

/**
 * Returns the matched node ids together with the ancestors needed to
 * reach them. `ancestors` never contains a match itself so callers can
 * tell a real match from a row kept only as a path.
 */
export const collectMatchingChatIDs = (
	model: ChatTreeModel,
	predicate: (node: ChatTreeModelNode) => boolean,
): { matches: Set<string>; ancestors: Set<string> } => {
	const matches = new Set<string>();
	const ancestors = new Set<string>();
	for (const node of model.nodesById.values()) {
		if (predicate(node)) {
			matches.add(node.id);
		}
	}
	for (const id of matches) {
		let cursor = model.nodesById.get(id)?.parentId;
		while (cursor !== undefined && !ancestors.has(cursor)) {
			if (!matches.has(cursor)) {
				ancestors.add(cursor);
			}
			cursor = model.nodesById.get(cursor)?.parentId;
		}
	}
	return { matches, ancestors };
};

export const collectAncestorIDs = (
	model: ChatTreeModel,
	id: string,
): string[] => {
	const ancestors: string[] = [];
	let cursor = model.nodesById.get(id)?.parentId;
	while (cursor !== undefined && !ancestors.includes(cursor)) {
		ancestors.push(cursor);
		cursor = model.nodesById.get(cursor)?.parentId;
	}
	return ancestors;
};

/** Counts named (non subagent) descendants present in the model. */
export const countNamedDescendants = (
	model: ChatTreeModel,
	id: string,
): number => {
	let count = 0;
	const stack = [...getChatTreeChildren(model, id)];
	const seen = new Set<string>();
	while (stack.length > 0) {
		const current = stack.pop();
		if (current === undefined || seen.has(current)) {
			continue;
		}
		seen.add(current);
		const node = model.nodesById.get(current);
		if (!node || node.kind === "subagent") {
			continue;
		}
		count += 1;
		stack.push(...getChatTreeChildren(model, current));
	}
	return count;
};

/** Collects the statuses of every descendant chat present in the model. */
export const collectDescendantChats = (
	model: ChatTreeModel,
	id: string,
): Chat[] => {
	const chats: Chat[] = [];
	const stack = [...getChatTreeChildren(model, id)];
	const seen = new Set<string>();
	while (stack.length > 0) {
		const current = stack.pop();
		if (current === undefined || seen.has(current)) {
			continue;
		}
		seen.add(current);
		const node = model.nodesById.get(current);
		if (node?.chat) {
			chats.push(node.chat);
		}
		stack.push(...getChatTreeChildren(model, current));
	}
	return chats;
};

/**
 * A chat at the maximum depth cannot receive named children. Server
 * depth is absent on rows outside a rooted tree, which means the limit
 * cannot be known and creation stays allowed.
 */
export const isChatAtTreeDepthLimit = (chat: Chat): boolean =>
	chat.depth !== undefined && chat.depth >= ChatTreeMaxDepth;
