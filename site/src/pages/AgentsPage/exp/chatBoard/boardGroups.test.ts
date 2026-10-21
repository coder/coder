import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	boardSidebarChats,
	collectBoardGroups,
	groupUpdatedAt,
	isBoardGroupMember,
} from "./boardGroups";

const chat = (
	id: string,
	labels: Record<string, string> = {},
	updated_at = MockChat.updated_at,
): Chat => ({
	...MockChat,
	id,
	labels,
	updated_at,
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

	it("folds members into their primary's slot", () => {
		const primary = chat("p");
		const member = chat("m", { "board/group": "p" });
		const chats = [primary, member];

		const { chats: listed, groups } = boardSidebarChats(chats);
		expect(listed).toEqual([primary]);
		expect(groups.get("p")).toEqual([member]);
	});

	it("places a group by its most recently updated chat", () => {
		const member = chat("m", { "board/group": "p" }, "2026-01-03T00:00:00Z");
		const other = chat("o", {}, "2026-01-02T00:00:00Z");
		const primary = chat("p", {}, "2026-01-01T00:00:00Z");

		const { chats: listed, groups } = boardSidebarChats([
			member,
			other,
			primary,
		]);
		expect(listed.map((c) => c.id)).toEqual(["p", "o"]);
		expect(groupUpdatedAt(primary, groups)).toBe(member.updated_at);
	});
});
