import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useBoardStorage } from "./boardStorage";

const KEY = "agents.board";

const pinned = {
	chatId: "a",
	x: 10,
	y: 20,
	width: 400,
	height: 300,
	pinned: true,
};

describe("useBoardStorage", () => {
	afterEach(() => {
		localStorage.clear();
	});

	it("starts empty without stored state", () => {
		const { result } = renderHook(() => useBoardStorage());
		expect(result.current[0]).toEqual({
			columnOrder: [],
			emptyColumns: [],
			windows: [],
		});
	});

	it("keeps valid entries and drops unpinned or malformed windows", () => {
		localStorage.setItem(
			KEY,
			JSON.stringify({
				columnOrder: ["Inbox", "Done"],
				emptyColumns: ["Later", 7],
				windows: [
					pinned,
					{ ...pinned, chatId: "preview", pinned: false },
					{ ...pinned, chatId: "broken", width: "wide" },
					"garbage",
				],
			}),
		);
		const { result } = renderHook(() => useBoardStorage());
		expect(result.current[0]).toEqual({
			columnOrder: ["Inbox", "Done"],
			emptyColumns: [],
			windows: [pinned],
		});
	});

	it("falls back to defaults on unreadable storage", () => {
		localStorage.setItem(KEY, "{not json");
		const { result } = renderHook(() => useBoardStorage());
		expect(result.current[0].columnOrder).toEqual([]);
	});

	it("persists object and functional patches", () => {
		const { result } = renderHook(() => useBoardStorage());
		act(() => result.current[1]({ columnOrder: ["Inbox", "Doing"] }));
		act(() =>
			result.current[1]((prev) => ({
				emptyColumns: [...prev.emptyColumns, "Later"],
			})),
		);
		expect(result.current[0].columnOrder).toEqual(["Inbox", "Doing"]);
		expect(result.current[0].emptyColumns).toEqual(["Later"]);
		expect(JSON.parse(localStorage.getItem(KEY) ?? "{}")).toEqual({
			columnOrder: ["Inbox", "Doing"],
			emptyColumns: ["Later"],
			windows: [],
		});
	});
});
