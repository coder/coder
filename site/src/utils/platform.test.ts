import { isLinux, isMac, isWindows, supportsCoderDesktop } from "./platform";

const setPlatform = (value: string) => {
	Object.defineProperty(window.navigator, "platform", {
		value,
		configurable: true,
	});
};

const setMaxTouchPoints = (value: number) => {
	Object.defineProperty(window.navigator, "maxTouchPoints", {
		value,
		configurable: true,
	});
};

describe("platform", () => {
	const originalPlatform = navigator.platform;
	const originalMaxTouchPoints = navigator.maxTouchPoints;

	afterEach(() => {
		setPlatform(originalPlatform);
		setMaxTouchPoints(originalMaxTouchPoints);
	});

	it("detects macOS", () => {
		setPlatform("MacIntel");
		setMaxTouchPoints(0);
		expect(isMac()).toBe(true);
		expect(isWindows()).toBe(false);
		expect(isLinux()).toBe(false);
		expect(supportsCoderDesktop()).toBe(true);
	});

	it("detects Windows", () => {
		setPlatform("Win32");
		expect(isWindows()).toBe(true);
		expect(isMac()).toBe(false);
		expect(isLinux()).toBe(false);
		expect(supportsCoderDesktop()).toBe(true);
	});

	it("detects Linux", () => {
		setPlatform("Linux x86_64");
		expect(isLinux()).toBe(true);
		expect(isMac()).toBe(false);
		expect(isWindows()).toBe(false);
		expect(supportsCoderDesktop()).toBe(true);
	});

	it("treats other platforms as unsupported", () => {
		setPlatform("CrOS x86_64");
		expect(isMac()).toBe(false);
		expect(isWindows()).toBe(false);
		expect(isLinux()).toBe(false);
		expect(supportsCoderDesktop()).toBe(false);
	});

	it("excludes iPadOS masquerading as macOS", () => {
		// iPadOS 13+ reports "MacIntel" but exposes a touchscreen.
		setPlatform("MacIntel");
		setMaxTouchPoints(5);
		expect(isMac()).toBe(true);
		expect(supportsCoderDesktop()).toBe(false);
	});
});
