import { afterEach, describe, expect, it } from "vitest";
import {
	type ChatWindow,
	readBoardStorage,
	saveBoardStorage,
} from "./boardStorage";

const KEY = "agents.board";

const pinned: ChatWindow = {
	kind: "chat",
	chatId: "a",
	x: 10,
	y: 20,
	width: 400,
	height: 300,
	pinned: true,
};

describe("boardStorage", () => {
	afterEach(() => {
		localStorage.clear();
	});

	it("starts empty without stored state", () => {
		expect(readBoardStorage()).toEqual({
			columnOrder: [],
			emptyColumns: [],
			windows: [],
		});
	});

	it("keeps valid entries and drops unpinned, draft or malformed windows", () => {
		localStorage.setItem(
			KEY,
			JSON.stringify({
				columnOrder: ["Inbox", "Done"],
				emptyColumns: ["Later", 7],
				windows: [
					pinned,
					{ ...pinned, chatId: "preview", pinned: false },
					{ ...pinned, chatId: "broken", width: "wide" },
					{
						...pinned,
						kind: "draft",
						chatId: undefined,
						target: { column: "Done" },
						withContext: false,
					},
					"garbage",
				],
			}),
		);
		expect(readBoardStorage()).toEqual({
			columnOrder: ["Inbox", "Done"],
			emptyColumns: [],
			windows: [pinned],
		});
	});

	it("falls back to defaults on unreadable storage", () => {
		localStorage.setItem(KEY, "{not json");
		expect(readBoardStorage().columnOrder).toEqual([]);
	});

	it("round-trips what was saved", () => {
		const next = {
			columnOrder: ["Inbox", "Doing"],
			emptyColumns: ["Later"],
			windows: [pinned],
		};
		saveBoardStorage(next);
		expect(readBoardStorage()).toEqual(next);
	});
});
