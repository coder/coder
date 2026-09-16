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
	});
});
