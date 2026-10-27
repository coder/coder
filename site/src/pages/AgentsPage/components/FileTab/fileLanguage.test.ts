import { describe, expect, it } from "vitest";
import { getFileLanguage } from "./fileLanguage";

describe("getFileLanguage", () => {
	it.each([
		["/repo/main.go", "Go"],
		["/repo/README.MD", "Markdown"],
		["/repo/site/src/api.d.ts", "TypeScript"],
		["/repo/Dockerfile", "Dockerfile"],
		["/home/coder/.ssh/config", "Ini"],
		["/repo/go.mod", undefined],
		["/repo/Makefile", undefined],
	])("names the language of %s as %s", (path, language) => {
		expect(getFileLanguage(path)).toBe(language);
	});
});
