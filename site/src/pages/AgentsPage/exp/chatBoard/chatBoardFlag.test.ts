import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
	getChatBoardEnabled,
	saveChatBoardEnabled,
	useChatBoardEnabled,
} from "./chatBoardFlag";

describe("chatBoardFlag", () => {
	afterEach(() => {
		localStorage.clear();
	});

	it("is off until saved, and every reader sees the change", () => {
		const { result } = renderHook(() => useChatBoardEnabled());
		expect(result.current).toBe(false);
		expect(getChatBoardEnabled()).toBe(false);

		act(() => saveChatBoardEnabled(true));

		expect(result.current).toBe(true);
		expect(getChatBoardEnabled()).toBe(true);
		expect(localStorage.getItem("agents.exp.chat-board")).toBe("true");

		act(() => saveChatBoardEnabled(false));

		expect(result.current).toBe(false);
	});

	it("turns off when another tab clears storage", () => {
		saveChatBoardEnabled(true);
		const { result } = renderHook(() => useChatBoardEnabled());
		expect(result.current).toBe(true);

		localStorage.clear();
		act(() => {
			window.dispatchEvent(new StorageEvent("storage", { key: null }));
		});

		expect(result.current).toBe(false);
	});
});
