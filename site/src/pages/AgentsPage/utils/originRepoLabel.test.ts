import { describe, expect, it } from "vitest";
import { originRepoLabel } from "./originRepoLabel";

describe("originRepoLabel", () => {
	it("reads the owner and repository from a remote URL", () => {
		expect(originRepoLabel("https://github.com/coder/coder.git")).toBe(
			"coder/coder",
		);
	});

	it("strips the .git suffix", () => {
		expect(originRepoLabel("https://github.com/coder/coder")).toBe(
			"coder/coder",
		);
	});

	it("reads an scp-style remote", () => {
		expect(originRepoLabel("git@github.com:coder/coder.git")).toBe(
			"coder/coder",
		);
	});

	it("keeps the origin when it names no repository", () => {
		expect(originRepoLabel("https://example.com")).toBe("https://example.com");
	});

	it("returns an empty label for a missing origin", () => {
		expect(originRepoLabel(undefined)).toBe("");
	});
});
