import { beforeEach, describe, expect, it } from "vitest";
import {
	MockChatTreeChild,
	MockChatTreeRoot,
} from "#/testHelpers/chatEntities";
import {
	CHAT_TREE_EXPANSION_STORAGE_KEY,
	EMPTY_CHAT_TREE_EXPANSION,
	isChatTreeNodeExpanded,
	loadChatTreeExpansion,
	persistChatTreeExpansion,
	setChatTreeNodeExpanded,
	setChatTreeSubagentsShown,
} from "./chatTreeExpansion";
import type { ChatTreeModelNode } from "./chatTreeModel";

const rootNode: ChatTreeModelNode = {
	id: MockChatTreeRoot.id,
	kind: "root",
	chat: MockChatTreeRoot,
	organizationId: "org-1",
	label: "Root",
	level: 1,
};

const childNode: ChatTreeModelNode = {
	id: MockChatTreeChild.id,
	kind: "chat",
	chat: MockChatTreeChild,
	organizationId: "org-1",
	label: MockChatTreeChild.title,
	level: 2,
	parentId: rootNode.id,
};

const organizationNode: ChatTreeModelNode = {
	id: "organization:org-1",
	kind: "organization",
	organizationId: "org-1",
	label: "Acme",
	level: 1,
};

beforeEach(() => {
	localStorage.clear();
});

describe("chat tree expansion state", () => {
	it("starts with roots and organizations expanded and other nodes collapsed", () => {
		expect(isChatTreeNodeExpanded(EMPTY_CHAT_TREE_EXPANSION, rootNode)).toBe(
			true,
		);
		expect(
			isChatTreeNodeExpanded(EMPTY_CHAT_TREE_EXPANSION, organizationNode),
		).toBe(true);
		expect(isChatTreeNodeExpanded(EMPTY_CHAT_TREE_EXPANSION, childNode)).toBe(
			false,
		);
	});

	it("records collapses for top-level nodes and expands for the rest", () => {
		let state = setChatTreeNodeExpanded(
			EMPTY_CHAT_TREE_EXPANSION,
			rootNode,
			false,
		);
		state = setChatTreeNodeExpanded(state, childNode, true);
		expect(isChatTreeNodeExpanded(state, rootNode)).toBe(false);
		expect(isChatTreeNodeExpanded(state, childNode)).toBe(true);

		state = setChatTreeNodeExpanded(state, rootNode, true);
		state = setChatTreeNodeExpanded(state, childNode, false);
		expect(state.collapsedTopLevel.size).toBe(0);
		expect(state.expanded.size).toBe(0);
	});

	it("returns the same state when nothing changes", () => {
		const state = setChatTreeNodeExpanded(
			EMPTY_CHAT_TREE_EXPANSION,
			childNode,
			false,
		);
		expect(state).toBe(EMPTY_CHAT_TREE_EXPANSION);
		expect(
			setChatTreeSubagentsShown(EMPTY_CHAT_TREE_EXPANSION, childNode.id, false),
		).toBe(EMPTY_CHAT_TREE_EXPANSION);
	});

	it("round-trips through localStorage", () => {
		let state = setChatTreeNodeExpanded(
			EMPTY_CHAT_TREE_EXPANSION,
			childNode,
			true,
		);
		state = setChatTreeNodeExpanded(state, rootNode, false);
		state = setChatTreeSubagentsShown(state, childNode.id, true);
		persistChatTreeExpansion(state);

		const loaded = loadChatTreeExpansion();
		expect([...loaded.expanded]).toEqual([childNode.id]);
		expect([...loaded.collapsedTopLevel]).toEqual([rootNode.id]);
		expect([...loaded.subagents]).toEqual([childNode.id]);
	});

	it("falls back to defaults on missing or corrupt storage", () => {
		expect(loadChatTreeExpansion()).toEqual(EMPTY_CHAT_TREE_EXPANSION);

		localStorage.setItem(CHAT_TREE_EXPANSION_STORAGE_KEY, "{not json");
		expect(loadChatTreeExpansion()).toEqual(EMPTY_CHAT_TREE_EXPANSION);

		localStorage.setItem(
			CHAT_TREE_EXPANSION_STORAGE_KEY,
			JSON.stringify({ expanded: [1, "ok", null], subagents: "nope" }),
		);
		const loaded = loadChatTreeExpansion();
		expect([...loaded.expanded]).toEqual(["ok"]);
		expect(loaded.subagents.size).toBe(0);
	});
});
