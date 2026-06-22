import { describe, expect, it } from "vitest";
import { getPathBasename } from "./path";

describe("getPathBasename", () => {
	it.each([
		["foo/bar.ts", "bar.ts"],
		["main.go", "main.go"],
		["", ""],
		["dir/", "dir/"],
		["/", "/"],
	])("returns the basename for %s", (path, expected) => {
		expect(getPathBasename(path)).toBe(expected);
	});
});
