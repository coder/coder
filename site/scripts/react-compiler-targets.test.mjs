import { describe, expect, it } from "vitest";
import {
	reactCompilerInclude,
	reactCompilerTargetDirs,
} from "./react-compiler-targets.mjs";

describe("React Compiler targets", () => {
	it("keeps the configured directories explicit", () => {
		expect(reactCompilerTargetDirs).toEqual([
			"src/pages/AgentsPage",
			"src/pages/AIBridgePage",
		]);
	});

	it.each(reactCompilerTargetDirs)("includes files under %s", (dir) => {
		const file = `/home/coder/coder/site/${dir}/Page.tsx`;
		expect(reactCompilerInclude.some((pattern) => pattern.test(file))).toBe(true);
	});

	it("does not include other page directories", () => {
		const file = "/home/coder/coder/site/src/pages/TemplatesPage/TemplatesPage.tsx";
		expect(reactCompilerInclude.some((pattern) => pattern.test(file))).toBe(false);
	});
});
