import { renderHook } from "@testing-library/react";
import { isValidElement } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { ChatTreeNode } from "../../components/ChatsSidebar/tree/ChatTreeNode";
import { BoardGroupEntry } from "./BoardGroupEntry";
import { useChatBoardSidebar } from "./useChatBoardSidebar";

const primary: Chat = { ...MockChat, id: "primary", labels: {} };
const member: Chat = {
	...MockChat,
	id: "member",
	labels: { "board/group": "primary" },
};
const chats = [primary, member];

const elementOf = (node: unknown) => {
	if (!isValidElement(node)) {
		throw new Error("expected a React element");
	}
	return node;
};

describe("useChatBoardSidebar", () => {
	afterEach(() => {
		localStorage.clear();
	});

	it("renders the plain list while the board is off", () => {
		const { result } = renderHook(() => useChatBoardSidebar(chats));

		expect(result.current.chats).toBe(chats);
		expect(result.current.renderTrailing).toBeUndefined();
		const entry = elementOf(result.current.renderEntry(primary));
		expect(entry.type).toBe(ChatTreeNode);
		expect(entry.props).toEqual({ chat: primary });
	});

	it("folds members into their primary's entry while on", () => {
		localStorage.setItem("agents.exp.chat-board", "true");
		const { result } = renderHook(() => useChatBoardSidebar(chats));

		expect(result.current.chats).toEqual([primary]);
		const entry = elementOf(result.current.renderEntry(primary));
		expect(entry.type).toBe(BoardGroupEntry);
		expect(entry.props).toEqual({ chat: primary, members: [member] });
	});
});
