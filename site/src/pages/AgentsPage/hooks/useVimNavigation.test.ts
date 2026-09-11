import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { isMac } from "#/utils/platform";
import {
	useVimNavigationModifier,
	VIM_NAVIGATION_MODIFIER_STORAGE_KEY,
} from "./useVimNavigation";

vi.mock("#/utils/platform", async (importOriginal) => ({
	...(await importOriginal<typeof import("#/utils/platform")>()),
	isMac: vi.fn(),
}));

const isMacMock = vi.mocked(isMac);

describe("useVimNavigationModifier", () => {
	afterEach(() => {
		localStorage.removeItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY);
	});

	it("defaults to Ctrl when nothing is stored", () => {
		isMacMock.mockReturnValue(false);
		const { result } = renderHook(() => useVimNavigationModifier());
		expect(result.current[0]).toBe("ctrl");
	});

	it("defaults to Cmd on macOS", () => {
		isMacMock.mockReturnValue(true);
		const { result } = renderHook(() => useVimNavigationModifier());
		expect(result.current[0]).toBe("meta");
	});

	it("falls back to the default for an unrecognized stored value", () => {
		isMacMock.mockReturnValue(false);
		localStorage.setItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY, "hyper");
		const { result } = renderHook(() => useVimNavigationModifier());
		expect(result.current[0]).toBe("ctrl");
	});

	it("persists and publishes a new modifier", () => {
		isMacMock.mockReturnValue(false);
		const { result } = renderHook(() => useVimNavigationModifier());

		act(() => {
			result.current[1]("alt");
		});

		expect(result.current[0]).toBe("alt");
		expect(localStorage.getItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY)).toBe(
			"alt",
		);
	});
});
