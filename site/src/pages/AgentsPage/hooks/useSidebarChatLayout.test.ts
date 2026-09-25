import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
	sidebarChatLayoutStorageKey,
	useSidebarChatLayout,
} from "./useSidebarChatLayout";

describe("useSidebarChatLayout", () => {
	afterEach(() => {
		localStorage.removeItem(sidebarChatLayoutStorageKey);
	});

	it("defaults to two lines", () => {
		const { result } = renderHook(() => useSidebarChatLayout());
		expect(result.current[0]).toBe("two_line");
	});

	it("persists the layout and updates every consumer in the tab", () => {
		const settings = renderHook(() => useSidebarChatLayout());
		const sidebar = renderHook(() => useSidebarChatLayout());

		act(() => settings.result.current[1]("one_line"));

		expect(localStorage.getItem(sidebarChatLayoutStorageKey)).toBe("one_line");
		expect(sidebar.result.current[0]).toBe("one_line");
	});

	it("falls back to two lines for an unknown stored value", () => {
		localStorage.setItem(sidebarChatLayoutStorageKey, "compact");
		const { result } = renderHook(() => useSidebarChatLayout());
		expect(result.current[0]).toBe("two_line");
	});
});
