import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	saveChatBoardOptIn,
	useChatBoardEnabled,
	useChatBoardOptIn,
} from "./chatBoardFlag";

const dashboard = vi.hoisted(() => ({ experiments: [] as string[] }));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ experiments: dashboard.experiments }),
}));

describe("chatBoardFlag", () => {
	afterEach(() => {
		localStorage.clear();
		dashboard.experiments = [];
	});

	it("is off until saved, and every reader sees the change", () => {
		const { result } = renderHook(() => useChatBoardOptIn());
		expect(result.current).toBe(false);

		act(() => saveChatBoardOptIn(true));

		expect(result.current).toBe(true);
		expect(localStorage.getItem("agents.exp.chat-board")).toBe("true");

		act(() => saveChatBoardOptIn(false));

		expect(result.current).toBe(false);
	});

	it("turns off when another tab clears storage", () => {
		saveChatBoardOptIn(true);
		const { result } = renderHook(() => useChatBoardOptIn());
		expect(result.current).toBe(true);

		localStorage.clear();
		act(() => {
			window.dispatchEvent(new StorageEvent("storage", { key: null }));
		});

		expect(result.current).toBe(false);
	});

	it("enables the board only with the deployment experiment and the opt-in", () => {
		const { result, rerender } = renderHook(() => useChatBoardEnabled());
		expect(result.current).toBe(false);

		act(() => saveChatBoardOptIn(true));
		expect(result.current).toBe(false);

		// The opt-in saved while the experiment was off still applies.
		dashboard.experiments = ["chat-board"];
		rerender();
		expect(result.current).toBe(true);

		act(() => saveChatBoardOptIn(false));
		expect(result.current).toBe(false);
	});
});
