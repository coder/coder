import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { buildChatTree } from "./chatTree";
import { getVisibleChatOrder } from "./visibleChatOrder";

const chat = (id: string, children: Chat[] = []): Chat =>
	({ id, title: id, children }) as unknown as Chat;

describe("getVisibleChatOrder", () => {
	const child = chat("a1");
	const a = chat("a", [child]);
	const b = chat("b");
	const c = chat("c");
	const tree = buildChatTree([a, b, c]);
	const sections = [
		{ key: "Pinned", chats: [c] },
		{ key: "Today", chats: [a, b] },
	];

	it("lists sections in order and skips collapsed children", () => {
		expect(
			getVisibleChatOrder({
				sections,
				collapsedSections: {},
				expandedById: {},
				tree,
			}),
		).toEqual({ visible: ["c", "a", "b"], all: ["c", "a", "a1", "b"] });
	});

	it("includes children of expanded roots after the root", () => {
		expect(
			getVisibleChatOrder({
				sections,
				collapsedSections: {},
				expandedById: { a: true },
				tree,
			}).visible,
		).toEqual(["c", "a", "a1", "b"]);
	});

	it("omits chats in collapsed sections", () => {
		expect(
			getVisibleChatOrder({
				sections,
				collapsedSections: { Pinned: true },
				expandedById: {},
				tree,
			}),
		).toEqual({ visible: ["a", "b"], all: ["c", "a", "a1", "b"] });
	});

	it("omits children of an expanded root inside a collapsed section", () => {
		expect(
			getVisibleChatOrder({
				sections,
				collapsedSections: { Today: true },
				expandedById: { a: true },
				tree,
			}).visible,
		).toEqual(["c"]);
	});
});
