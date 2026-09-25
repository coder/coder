import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
	agentFontSizeStorageKey,
	useAgentFontSize,
	useApplyAgentFontSize,
} from "./useAgentFontSize";

const appliedFontSize = () =>
	document.documentElement.style.getPropertyValue("--agent-font-size");

describe("useAgentFontSize", () => {
	afterEach(() => {
		localStorage.removeItem(agentFontSizeStorageKey);
	});

	it("defaults to 14px", () => {
		const { result } = renderHook(() => useAgentFontSize());
		expect(result.current[0]).toBe("14");
	});

	it("falls back to 14px for an unknown stored value", () => {
		localStorage.setItem(agentFontSizeStorageKey, "20");
		const { result } = renderHook(() => useAgentFontSize());
		expect(result.current[0]).toBe("14");
	});

	it("applies the chosen size to the document and clears it on unmount", () => {
		const shell = renderHook(() => useApplyAgentFontSize());
		const settings = renderHook(() => useAgentFontSize());
		expect(appliedFontSize()).toBe("14px");

		act(() => settings.result.current[1]("13"));

		expect(localStorage.getItem(agentFontSizeStorageKey)).toBe("13");
		expect(appliedFontSize()).toBe("13px");

		shell.unmount();
		expect(appliedFontSize()).toBe("");
	});
});
