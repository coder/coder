import { afterEach, describe, expect, it, vi } from "vitest";
import { isMac, isWindows, supportsCoderDesktop } from "./platform";

const stubPlatform = (platform: string, maxTouchPoints = 0) => {
	vi.stubGlobal("navigator", {
		...navigator,
		platform,
		maxTouchPoints,
	});
};

describe("platform", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("detects macOS", () => {
		stubPlatform("MacIntel");
		expect(isMac()).toBe(true);
		expect(isWindows()).toBe(false);
		expect(supportsCoderDesktop()).toBe(true);
	});

	it("detects Windows", () => {
		stubPlatform("Win32");
		expect(isWindows()).toBe(true);
		expect(isMac()).toBe(false);
		expect(supportsCoderDesktop()).toBe(true);
	});

	it("treats Linux and other platforms as unsupported", () => {
		stubPlatform("Linux x86_64");
		expect(isMac()).toBe(false);
		expect(isWindows()).toBe(false);
		expect(supportsCoderDesktop()).toBe(false);
	});

	it("excludes iPadOS masquerading as macOS", () => {
		// iPadOS 13+ reports "MacIntel" but exposes a touchscreen.
		stubPlatform("MacIntel", 5);
		expect(isMac()).toBe(true);
		expect(supportsCoderDesktop()).toBe(false);
	});
});
