import { describe, expect, it } from "vitest";
import { parsePullRequestUrl } from "./pullRequest";

describe("parsePullRequestUrl", () => {
	it("parses canonical GitHub pull request URLs", () => {
		expect(
			parsePullRequestUrl("https://github.com/coder/coder/pull/42"),
		).toEqual({
			owner: "coder",
			repo: "coder",
			number: "42",
		});
	});

	it("parses pull request URLs behind enterprise path prefixes", () => {
		expect(
			parsePullRequestUrl("https://git.example.com/git/org/repo/pull/42"),
		).toEqual({
			owner: "org",
			repo: "repo",
			number: "42",
		});
	});

	it("parses pull request URLs with suffix pages", () => {
		expect(
			parsePullRequestUrl("https://github.com/coder/coder/pull/42/files"),
		).toEqual({
			owner: "coder",
			repo: "coder",
			number: "42",
		});
	});

	it("parses enterprise pull request URLs with suffix pages", () => {
		expect(
			parsePullRequestUrl("https://git.example.com/git/org/repo/pull/42/files"),
		).toEqual({
			owner: "org",
			repo: "repo",
			number: "42",
		});
	});

	it("ignores branch URLs that only contain pull-like path segments", () => {
		expect(
			parsePullRequestUrl(
				"https://github.com/coder/coder/tree/feature/pull/123/fix",
			),
		).toBeNull();
	});

	it("ignores non-pull request repository pages", () => {
		expect(
			parsePullRequestUrl(
				"https://git.example.com/git/org/repo/compare/main...feature",
			),
		).toBeNull();
	});
});
