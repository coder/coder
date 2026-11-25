import { describe, expect, it } from "vitest";
import { originRepoLabel } from "./originRepoLabel";

describe("originRepoLabel", () => {
	it.each([
		["https://github.com/coder/coder.git", "coder/coder"],
		["https://github.com/coder/coder", "coder/coder"],
		["https://github.com/coder/coder.git/", "coder/coder"],
		["ssh://git@github.com:2222/coder/coder.git", "coder/coder"],
		["git@github.com:coder/coder.git", "coder/coder"],
		["github.com:coder/coder.git", "coder/coder"],
		["git@github.com:coder.git", "coder"],
		["https://github.com/foo.git/bar", "foo.git/bar"],
		["https://gitlab.com/group/sub/repo.git", "group/sub/repo"],
		["https://dev.azure.com/org/project/_git/repo", "org/project/_git/repo"],
		["https://bitbucket.org/team/repo.git", "team/repo"],
		["/home/coder/repo", "home/coder/repo"],
		["https://example.com", "https://example.com"],
	])("labels %s as %s", (origin, label) => {
		expect(originRepoLabel(origin)).toBe(label);
	});
});
