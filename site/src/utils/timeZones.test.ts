import { afterEach, describe, expect, it, vi } from "vitest";
import { getPreferredTimezone } from "./timeZones";

const stubBrowserTimeZone = (timeZone: string) => {
	const resolvedOptions = Intl.DateTimeFormat.prototype.resolvedOptions;
	vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockImplementation(
		function (this: Intl.DateTimeFormat) {
			return { ...resolvedOptions.call(this), timeZone };
		},
	);
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("getPreferredTimezone", () => {
	it.each([
		{ browser: "Europe/Berlin", expected: "Europe/Berlin" },
		{ browser: "Etc/Unknown", expected: "UTC" },
	])(
		"returns $expected for the browser zone $browser",
		({ browser, expected }) => {
			stubBrowserTimeZone(browser);
			expect(getPreferredTimezone()).toBe(expected);
		},
	);
});
