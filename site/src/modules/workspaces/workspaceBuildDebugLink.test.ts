import { afterEach, describe, expect, it, vi } from "vitest";
import {
	buildDebugWorkspaceBuildPath,
	debugWorkspaceBuildIntentMaxAgeMs,
	debugWorkspaceBuildIntentStorageKey,
	debugWorkspaceBuildSearchParam,
	storeDebugWorkspaceBuildIntent,
	takeDebugWorkspaceBuildIntent,
} from "./workspaceBuildDebugLink";

afterEach(() => {
	localStorage.clear();
	vi.restoreAllMocks();
});

describe("buildDebugWorkspaceBuildPath", () => {
	it("links to the agents create page with the build id", () => {
		const path = buildDebugWorkspaceBuildPath("build id/with?chars");
		const url = new URL(path, "https://coder.example.com");

		expect(url.pathname).toBe("/agents");
		expect(url.searchParams.get(debugWorkspaceBuildSearchParam)).toBe(
			"build id/with?chars",
		);
	});
});

describe("takeDebugWorkspaceBuildIntent", () => {
	it("consumes the intent for the clicked build only", () => {
		storeDebugWorkspaceBuildIntent("build-a");

		expect(takeDebugWorkspaceBuildIntent("build-b")).toBe(false);
		expect(
			localStorage.getItem(debugWorkspaceBuildIntentStorageKey),
		).not.toBeNull();

		expect(takeDebugWorkspaceBuildIntent("build-a")).toBe(true);
		expect(
			localStorage.getItem(debugWorkspaceBuildIntentStorageKey),
		).toBeNull();
		expect(takeDebugWorkspaceBuildIntent("build-a")).toBe(false);
	});

	it("expires", () => {
		const now = Date.now();
		vi.spyOn(Date, "now").mockReturnValue(now);
		storeDebugWorkspaceBuildIntent("build-a");
		vi.spyOn(Date, "now").mockReturnValue(
			now + debugWorkspaceBuildIntentMaxAgeMs,
		);

		expect(takeDebugWorkspaceBuildIntent("build-a")).toBe(false);
	});

	it("ignores malformed storage", () => {
		localStorage.setItem(debugWorkspaceBuildIntentStorageKey, "{not json");

		expect(takeDebugWorkspaceBuildIntent("build-a")).toBe(false);
	});
});
