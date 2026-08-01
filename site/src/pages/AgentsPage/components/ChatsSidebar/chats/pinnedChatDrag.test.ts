import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { resolvePinnedChatDrop } from "./pinnedChatDrag";

const buildPinnedChat = (id: string, pinOrder: number): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	pin_order: pinOrder,
});

const pinnedChats = [
	buildPinnedChat("chat-1", 1),
	buildPinnedChat("chat-2", 2),
	buildPinnedChat("chat-3", 3),
];

describe("resolvePinnedChatDrop", () => {
	it("returns undefined when either end of the drop is not pinned", () => {
		expect(
			resolvePinnedChatDrop({
				pinnedChats,
				hasLocalOrder: false,
				activeId: "chat-unpinned",
				overId: "chat-1",
			}),
		).toBeUndefined();
		expect(
			resolvePinnedChatDrop({
				pinnedChats,
				hasLocalOrder: false,
				activeId: "chat-1",
				overId: "chat-unpinned",
			}),
		).toBeUndefined();
	});

	it("renumbers from the rendered order and leaves cache-fresh pin orders alone", () => {
		// Server-assigned pin orders can hold gaps, which are the freshest
		// values available while no drop is pending.
		const gapped = [
			buildPinnedChat("chat-1", 2),
			buildPinnedChat("chat-2", 5),
			buildPinnedChat("chat-3", 9),
		];

		const drop = resolvePinnedChatDrop({
			pinnedChats: gapped,
			hasLocalOrder: false,
			activeId: "chat-3",
			overId: "chat-1",
		});

		expect(drop).toEqual({
			localOrder: ["chat-3", "chat-1", "chat-2"],
			chatId: "chat-3",
			pinOrder: 1,
			mutationChats: gapped,
		});
	});

	it("normalizes the pre-drop order while a local drag order is active", () => {
		// What the panel renders after a first drop the parent has not
		// rerendered with: the position is new, the pin_order fields are
		// the ones the prop was built with.
		const locallyOrdered = [
			buildPinnedChat("chat-3", 3),
			buildPinnedChat("chat-1", 1),
			buildPinnedChat("chat-2", 2),
		];

		const drop = resolvePinnedChatDrop({
			pinnedChats: locallyOrdered,
			hasLocalOrder: true,
			activeId: "chat-2",
			overId: "chat-3",
		});

		expect(drop?.localOrder).toEqual(["chat-2", "chat-3", "chat-1"]);
		expect(drop?.chatId).toBe("chat-2");
		expect(drop?.pinOrder).toBe(1);
		// Pre-drop positions, so the mutation's rollback snapshot is the
		// order the user is looking at rather than the stale prop's.
		expect(
			drop?.mutationChats.map((chat) => [chat.id, chat.pin_order]),
		).toEqual([
			["chat-3", 1],
			["chat-1", 2],
			["chat-2", 3],
		]);
	});

	it("keeps the two drops of an un-rerendered sequence consistent", () => {
		const first = resolvePinnedChatDrop({
			pinnedChats,
			hasLocalOrder: false,
			activeId: "chat-3",
			overId: "chat-1",
		});
		expect(first?.pinOrder).toBe(1);
		expect(first?.localOrder).toEqual(["chat-3", "chat-1", "chat-2"]);

		// The parent still holds the pre-drop chats, so the panel renders
		// its local order over them and the second drop is renumbered
		// against that, not against the stale prop order.
		const rendered = (first?.localOrder ?? []).map(
			(id) => pinnedChats.find((chat) => chat.id === id) as Chat,
		);
		const second = resolvePinnedChatDrop({
			pinnedChats: rendered,
			hasLocalOrder: true,
			activeId: "chat-1",
			overId: "chat-2",
		});

		expect(second?.chatId).toBe("chat-1");
		expect(second?.pinOrder).toBe(3);
		expect(second?.localOrder).toEqual(["chat-3", "chat-2", "chat-1"]);
		expect(
			second?.mutationChats.map((chat) => [chat.id, chat.pin_order]),
		).toEqual([
			["chat-3", 1],
			["chat-1", 2],
			["chat-2", 3],
		]);
	});
});
