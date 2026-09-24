import { afterEach, describe, expect, it } from "vitest";
import {
	debugWorkspaceBuildIntentStorageKey,
	storeDebugWorkspaceBuildIntent,
	takeDebugWorkspaceBuildIntent,
} from "./workspaceBuildDebugLink";

afterEach(() => {
	localStorage.clear();
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
