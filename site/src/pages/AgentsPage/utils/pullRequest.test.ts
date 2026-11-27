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

	it("parses Gitea and Forgejo pull request URLs", () => {
		expect(
			parsePullRequestUrl(
				"https://git.example.com/forgejo/org/repo/pulls/42/files",
			),
		).toEqual({
			owner: "org",
			repo: "repo",
			number: "42",
		});
	});

	it("ignores Gitea branch URLs that contain pull-like path segments", () => {
		expect(
			parsePullRequestUrl(
				"https://git.example.com/org/repo/src/branch/feature/pulls/123",
			),
		).toBeNull();
	});

	it("ignores Gitea commit URLs that contain pull-like path segments", () => {
		expect(
			parsePullRequestUrl(
				"https://git.example.com/org/repo/src/commit/abc123/pulls/1",
			),
		).toBeNull();
	});

	it("parses pull request URLs whose repository is named src", () => {
		expect(parsePullRequestUrl("https://github.com/acme/src/pull/42")).toEqual({
			owner: "acme",
			repo: "src",
			number: "42",
		});
	});

	it("parses pull request URLs whose owner is named src", () => {
		expect(parsePullRequestUrl("https://github.com/src/repo/pull/7")).toEqual({
			owner: "src",
			repo: "repo",
			number: "7",
		});
	});

	it("parses pull request URLs behind a path prefix named src", () => {
		expect(
			parsePullRequestUrl("https://git.example.com/src/org/repo/pulls/3"),
		).toEqual({
			owner: "org",
			repo: "repo",
			number: "3",
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
