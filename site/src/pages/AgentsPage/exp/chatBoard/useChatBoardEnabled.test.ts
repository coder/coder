import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useChatBoardEnabled } from "./useChatBoardEnabled";

describe("useChatBoardEnabled", () => {
	afterEach(() => {
		localStorage.clear();
	});

	it("is off until enabled, then persists the choice", () => {
		const { result } = renderHook(() => useChatBoardEnabled());
		expect(result.current[0]).toBe(false);

		act(() => result.current[1](true));

		expect(result.current[0]).toBe(true);
		expect(localStorage.getItem("agents.exp.chat-board")).toBe("true");
		expect(renderHook(() => useChatBoardEnabled()).result.current[0]).toBe(
			true,
		);

		act(() => result.current[1](false));

		expect(result.current[0]).toBe(false);
		expect(localStorage.getItem("agents.exp.chat-board")).toBe("false");
	});

	it("turns off when another tab clears storage", () => {
		localStorage.setItem("agents.exp.chat-board", "true");
		const { result } = renderHook(() => useChatBoardEnabled());
		expect(result.current[0]).toBe(true);

		localStorage.clear();
		act(() => {
			window.dispatchEvent(new StorageEvent("storage", { key: null }));
		});

		expect(result.current[0]).toBe(false);
	});
});
