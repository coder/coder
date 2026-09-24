import { afterEach, describe, expect, it } from "vitest";
import {
	buildDebugWorkspaceBuildPath,
	debugWorkspaceBuildIntentStorageKey,
	debugWorkspaceBuildSearchParam,
	storeDebugWorkspaceBuildIntent,
	takeDebugWorkspaceBuildIntent,
} from "./workspaceBuildDebugLink";

afterEach(() => {
	localStorage.clear();
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
});
