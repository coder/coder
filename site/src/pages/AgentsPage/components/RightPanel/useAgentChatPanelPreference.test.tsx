import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocalStorageProvider } from "#/contexts/LocalStorageProvider";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import { RIGHT_PANEL_OPEN_KEY } from "./RightPanel";
import { useAgentChatPanelPreference } from "./useAgentChatPanelPreference";

const stubViewport = (narrow: boolean) => {
	vi.stubGlobal("matchMedia", (query: string) => ({
		matches: narrow && query === belowLgViewportMediaQuery,
		media: query,
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
	}));
};

const renderPreference = () =>
	renderHook(() => useAgentChatPanelPreference(), {
		wrapper: LocalStorageProvider,
	});

const writeFromAnotherTab = (value: string) => {
	act(() => {
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, value);
		window.dispatchEvent(
			new StorageEvent("storage", { key: RIGHT_PANEL_OPEN_KEY }),
		);
	});
};

describe("useAgentChatPanelPreference", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("shows the panel when the stored preference is open", () => {
		stubViewport(false);
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");

		const { result } = renderPreference();

		expect(result.current.showSidebarPanel).toBe(true);
	});

	it("follows the stored preference when another tab changes it", () => {
		stubViewport(false);
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");
		const { result } = renderPreference();

		writeFromAnotherTab("false");

		expect(result.current.showSidebarPanel).toBe(false);
	});

	it("hides the panel on a narrow viewport without changing the stored preference", () => {
		stubViewport(true);
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");

		const { result } = renderPreference();

		expect(result.current.showSidebarPanel).toBe(false);
		expect(localStorage.getItem(RIGHT_PANEL_OPEN_KEY)).toBe("true");
	});

	it("keeps narrow suppression when another tab updates the preference", () => {
		stubViewport(true);
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "false");
		const { result } = renderPreference();

		writeFromAnotherTab("true");

		expect(result.current.showSidebarPanel).toBe(false);
	});

	it("shows the panel on a narrow viewport after an explicit toggle", () => {
		stubViewport(true);
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "false");
		const { result } = renderPreference();

		act(() => result.current.handleSetShowSidebarPanel(true));

		expect(result.current.showSidebarPanel).toBe(true);
		expect(localStorage.getItem(RIGHT_PANEL_OPEN_KEY)).toBe("true");
	});
});
