import { isMac, isWindows, supportsCoderDesktop } from "./platform";

const setPlatform = (value: string) => {
	Object.defineProperty(window.navigator, "platform", {
		value,
		configurable: true,
	});
};

describe("platform", () => {
	const originalPlatform = navigator.platform;

	afterEach(() => {
		setPlatform(originalPlatform);
	});

	it("detects macOS", () => {
		setPlatform("MacIntel");
		expect(isMac()).toBe(true);
		expect(isWindows()).toBe(false);
		expect(supportsCoderDesktop()).toBe(true);
	});

	it("detects Windows", () => {
		setPlatform("Win32");
		expect(isWindows()).toBe(true);
		expect(isMac()).toBe(false);
		expect(supportsCoderDesktop()).toBe(true);
	});

	it("treats Linux and other platforms as unsupported", () => {
		setPlatform("Linux x86_64");
		expect(isMac()).toBe(false);
		expect(isWindows()).toBe(false);
		expect(supportsCoderDesktop()).toBe(false);
	});
});
