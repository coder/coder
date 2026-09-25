import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	boardSidebarChats,
	collectBoardGroups,
	isBoardGroupMember,
} from "./boardGroups";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	labels,
});

describe("boardGroups", () => {
	it("recognises a member by its group label pointing elsewhere", () => {
		expect(isBoardGroupMember(chat("a"))).toBe(false);
		expect(isBoardGroupMember(chat("a", { "board/group": "a" }))).toBe(false);
		expect(isBoardGroupMember(chat("a", { "board/group": "p" }))).toBe(true);
	});

	it("groups members only under a primary present in the list", () => {
		const primary = chat("p");
		const m1 = chat("m1", { "board/group": "p" });
		const m2 = chat("m2", { "board/group": "p" });
		const orphan = chat("o", { "board/group": "elsewhere" });
		const groups = collectBoardGroups([m1, primary, orphan, m2]);
		expect([...groups.keys()]).toEqual(["p"]);
		expect(groups.get("p")?.map((c) => c.id)).toEqual(["m1", "m2"]);
	});

	it("folds members into their primary's slot only while the board is on", () => {
		const primary = chat("p");
		const member = chat("m", { "board/group": "p" });
		const chats = [primary, member];

		expect(boardSidebarChats(chats, false)).toEqual({ chats, groups: null });

		const on = boardSidebarChats(chats, true);
		expect(on.chats).toEqual([primary]);
		expect(on.groups?.get("p")).toEqual([member]);
	});
});
