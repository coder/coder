import { afterEach, describe, expect, it } from "vitest";
import {
	type ChatWindow,
	readBoardStorage,
	saveBoardStorage,
} from "./boardStorage";

const USER = "user-a";
const KEY = `agents.board.${USER}`;

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
		expect(readBoardStorage(USER)).toEqual({
			columnOrder: [],
			emptyColumns: [],
			windows: [],
			effortFilter: null,
		});
	});

	it("keeps a stored effort filter and drops other values", () => {
		localStorage.setItem(KEY, JSON.stringify({ effortFilter: "Q3" }));
		expect(readBoardStorage(USER).effortFilter).toBe("Q3");
		localStorage.setItem(KEY, JSON.stringify({ effortFilter: 7 }));
		expect(readBoardStorage(USER).effortFilter).toBeNull();
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
		expect(readBoardStorage(USER)).toEqual({
			columnOrder: ["Inbox", "Done"],
			emptyColumns: [],
			windows: [pinned],
			effortFilter: null,
		});
	});

	it("falls back to defaults on unreadable storage", () => {
		localStorage.setItem(KEY, "{not json");
		expect(readBoardStorage(USER).columnOrder).toEqual([]);
	});

	it("round-trips what was saved", () => {
		const next = {
			columnOrder: ["Inbox", "Doing"],
			emptyColumns: ["Later"],
			windows: [pinned],
			effortFilter: "Q3",
		};
		saveBoardStorage(USER, next);
		expect(readBoardStorage(USER)).toEqual(next);
	});

	it("keeps each user's board apart", () => {
		saveBoardStorage(USER, {
			columnOrder: ["Inbox", "Doing"],
			emptyColumns: [],
			windows: [pinned],
			effortFilter: null,
		});
		expect(readBoardStorage("user-b")).toEqual({
			columnOrder: [],
			emptyColumns: [],
			windows: [],
			effortFilter: null,
		});
	});
});
