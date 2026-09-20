import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatTreeChild,
	MockChatTreeGrandchild,
	MockChatTreeResponse,
	MockChatTreeRoot,
	MockChatTreeSibling,
	MockChatTreeSubagent,
} from "#/testHelpers/chatEntities";
import {
	buildChatTreeModel,
	type ChatTreeModelNode,
	collectAncestorIDs,
	collectMatchingChatIDs,
	flattenVisibleTree,
	isChatAtTreeDepthLimit,
	organizationTreeNodeId,
} from "./chatTreeModel";

const org = { id: "org-1", displayName: "Acme" };
const otherOrg = { id: "org-2", displayName: "Globex" };

const singleOrganization = () =>
	buildChatTreeModel([{ organization: org, response: MockChatTreeResponse }]);

const allExpanded = () => true;
const expandedOnly = (ids: readonly string[]) => (node: ChatTreeModelNode) =>
	ids.includes(node.id);

describe(buildChatTreeModel.name, () => {
	it("places the root at the top and nests children by parent_chat_id", () => {
		const model = singleOrganization();
		expect(model.topLevelIds).toEqual([MockChatTreeRoot.id]);
		expect(model.childrenById.get(MockChatTreeRoot.id)).toEqual([
			MockChatTreeChild.id,
			MockChatTreeSibling.id,
		]);
		expect(model.childrenById.get(MockChatTreeChild.id)).toEqual([
			MockChatTreeGrandchild.id,
		]);
		expect(model.nodesById.get(MockChatTreeGrandchild.id)?.level).toBe(3);
		expect(model.nodesById.get(MockChatTreeRoot.id)?.kind).toBe("root");
	});

	it("attaches a row whose parent is missing under the root", () => {
		const model = buildChatTreeModel([
			{
				organization: org,
				response: {
					root_chat_id: MockChatTreeRoot.id,
					chats: [
						MockChatTreeRoot,
						{ ...MockChatTreeGrandchild, archived: true },
					],
				},
			},
		]);
		expect(model.childrenById.get(MockChatTreeRoot.id)).toEqual([
			MockChatTreeGrandchild.id,
		]);
		expect(model.nodesById.get(MockChatTreeGrandchild.id)?.level).toBe(2);
	});

	it("lists rows flat when the organization has no root", () => {
		const model = buildChatTreeModel([
			{
				organization: org,
				response: {
					root_chat_id: null,
					chats: [
						{
							...MockChatTreeChild,
							parent_chat_id: undefined,
							depth: undefined,
						},
						{
							...MockChatTreeSibling,
							parent_chat_id: undefined,
							depth: undefined,
						},
					],
				},
			},
		]);
		expect(model.topLevelIds).toEqual([
			MockChatTreeChild.id,
			MockChatTreeSibling.id,
		]);
		expect(model.nodesById.get(MockChatTreeChild.id)?.level).toBe(1);
	});

	it("adds a synthetic organization node above each root with several organizations", () => {
		const model = buildChatTreeModel([
			{ organization: org, response: MockChatTreeResponse },
			{
				organization: otherOrg,
				response: {
					root_chat_id: "root-2",
					chats: [
						{ ...MockChatTreeRoot, id: "root-2", organization_id: otherOrg.id },
					],
				},
			},
		]);
		expect(model.topLevelIds).toEqual([
			organizationTreeNodeId(org.id),
			organizationTreeNodeId(otherOrg.id),
		]);
		expect(model.nodesById.get(organizationTreeNodeId(org.id))).toMatchObject({
			kind: "organization",
			label: "Acme",
			level: 1,
		});
		expect(model.nodesById.get(MockChatTreeRoot.id)).toMatchObject({
			level: 2,
			parentId: organizationTreeNodeId(org.id),
		});
		expect(model.nodesById.get(MockChatTreeGrandchild.id)?.level).toBe(4);
	});

	it("appends fetched subagents after named children and drops subagent rows from the response", () => {
		const model = buildChatTreeModel([
			{
				organization: org,
				response: {
					...MockChatTreeResponse,
					chats: [...MockChatTreeResponse.chats, MockChatTreeSubagent],
				},
				subagentsByParent: new Map([
					[MockChatTreeChild.id, [MockChatTreeSubagent]],
				]),
			},
		]);
		expect(model.childrenById.get(MockChatTreeChild.id)).toEqual([
			MockChatTreeGrandchild.id,
			MockChatTreeSubagent.id,
		]);
		expect(model.nodesById.get(MockChatTreeSubagent.id)).toMatchObject({
			kind: "subagent",
			level: 3,
			parentId: MockChatTreeChild.id,
		});
	});

	it("keeps rows reachable when parent links form a cycle", () => {
		const a: Chat = { ...MockChat, id: "a", parent_chat_id: "b" };
		const b: Chat = { ...MockChat, id: "b", parent_chat_id: "a" };
		const model = buildChatTreeModel([
			{ organization: org, response: { root_chat_id: null, chats: [a, b] } },
		]);
		expect([...model.nodesById.keys()].sort()).toEqual(["a", "b"]);
		expect(model.topLevelIds).toEqual(["a"]);
		expect(model.childrenById.get("a")).toEqual(["b"]);
	});
});

describe(flattenVisibleTree.name, () => {
	it("returns only expanded rows with set positions", () => {
		const model = singleOrganization();
		const rows = flattenVisibleTree(model, {
			isExpanded: expandedOnly([MockChatTreeRoot.id]),
		});
		expect(rows.map((row) => row.id)).toEqual([
			MockChatTreeRoot.id,
			MockChatTreeChild.id,
			MockChatTreeSibling.id,
		]);
		expect(rows[1]).toMatchObject({
			hasChildren: true,
			isExpanded: false,
			setSize: 2,
			posInSet: 1,
		});
		expect(rows[2]).toMatchObject({ setSize: 2, posInSet: 2 });
	});

	it("includes descendants of expanded nodes in order", () => {
		const rows = flattenVisibleTree(singleOrganization(), {
			isExpanded: allExpanded,
		});
		expect(rows.map((row) => row.id)).toEqual([
			MockChatTreeRoot.id,
			MockChatTreeChild.id,
			MockChatTreeGrandchild.id,
			MockChatTreeSibling.id,
		]);
	});

	it("prunes to the visible set and recounts siblings", () => {
		const rows = flattenVisibleTree(singleOrganization(), {
			isExpanded: allExpanded,
			visible: new Set([
				MockChatTreeRoot.id,
				MockChatTreeChild.id,
				MockChatTreeGrandchild.id,
			]),
		});
		expect(rows.map((row) => row.id)).toEqual([
			MockChatTreeRoot.id,
			MockChatTreeChild.id,
			MockChatTreeGrandchild.id,
		]);
		expect(rows[1].setSize).toBe(1);
	});
});

describe(collectMatchingChatIDs.name, () => {
	it("returns matches and the ancestors needed to reach them", () => {
		const { matches, ancestors } = collectMatchingChatIDs(
			singleOrganization(),
			(node) => node.id === MockChatTreeGrandchild.id,
		);
		expect([...matches]).toEqual([MockChatTreeGrandchild.id]);
		expect([...ancestors].sort()).toEqual(
			[MockChatTreeChild.id, MockChatTreeRoot.id].sort(),
		);
	});

	it("does not list a matched node as an ancestor", () => {
		const { ancestors } = collectMatchingChatIDs(
			singleOrganization(),
			(node) =>
				node.id === MockChatTreeChild.id ||
				node.id === MockChatTreeGrandchild.id,
		);
		expect([...ancestors]).toEqual([MockChatTreeRoot.id]);
	});
});

describe("counts and limits", () => {
	it("walks ancestors from a node to the top", () => {
		expect(
			collectAncestorIDs(singleOrganization(), MockChatTreeGrandchild.id),
		).toEqual([MockChatTreeChild.id, MockChatTreeRoot.id]);
	});

	it("flags the depth limit only when server depth is known", () => {
		expect(isChatAtTreeDepthLimit({ ...MockChat, depth: 5 })).toBe(true);
		expect(isChatAtTreeDepthLimit({ ...MockChat, depth: 4 })).toBe(false);
		expect(isChatAtTreeDepthLimit({ ...MockChat, depth: undefined })).toBe(
			false,
		);
	});
});
