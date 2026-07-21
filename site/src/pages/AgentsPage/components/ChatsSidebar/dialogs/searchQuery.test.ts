import { describe, expect, it } from "vitest";
import { normalizeChatSearchInput } from "./searchQuery";

describe("normalizeChatSearchInput", () => {
	it("returns undefined for empty input", () => {
		expect(normalizeChatSearchInput("")).toBeUndefined();
		expect(normalizeChatSearchInput("   ")).toBeUndefined();
	});

	it("normalizes key:value filters", () => {
		expect(normalizeChatSearchInput("has_unread:true")).toBe("has_unread:true");
		expect(normalizeChatSearchInput('title:"chat title" archived:true')).toBe(
			'title:"chat title" archived:true',
		);
		expect(normalizeChatSearchInput("pr_status:open,merged")).toBe(
			"pr_status:open,merged",
		);
		expect(
			normalizeChatSearchInput(
				'diff_url:"https://github.com/coder/coder/pull/25391"',
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/25391"');
		expect(
			normalizeChatSearchInput(
				"diff_url:https://github.com/coder/coder/pull/26016",
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/26016"');
		expect(
			normalizeChatSearchInput("diff_url:github.com/coder/coder/pull/26016"),
		).toBe('diff_url:"https://github.com/coder/coder/pull/26016"');
		expect(
			normalizeChatSearchInput('diff_url:"github.com/coder/coder/pull/26016"'),
		).toBe('diff_url:"https://github.com/coder/coder/pull/26016"');
	});

	it("re-quotes passthrough values containing spaces so the result round-trips", () => {
		const normalized = normalizeChatSearchInput('pr_status:"open merged"');
		expect(normalized).toBe('pr_status:"open merged"');
		expect(normalizeChatSearchInput(normalized ?? "")).toBe(
			'pr_status:"open merged"',
		);
	});

	it("converts bare search text into a quoted FTS search filter", () => {
		expect(normalizeChatSearchInput("Fix")).toBe('search:"Fix"');
		expect(normalizeChatSearchInput("fix auth middleware")).toBe(
			'search:"fix auth middleware"',
		);
		expect(normalizeChatSearchInput("hello world")).toBe(
			'search:"hello world"',
		);
		expect(normalizeChatSearchInput("fix:lint")).toBe('search:"fix:lint"');
	});

	it("combines key:value filters with an FTS search fallback for bare text", () => {
		expect(normalizeChatSearchInput("has_unread:true fix auth")).toBe(
			'has_unread:true search:"fix auth"',
		);
		expect(normalizeChatSearchInput("archived:true fix:lint")).toBe(
			'archived:true search:"fix:lint"',
		);
		expect(normalizeChatSearchInput("fix has_unread:true auth")).toBe(
			'has_unread:true search:"fix auth"',
		);
		expect(
			normalizeChatSearchInput(
				"diff_url:https://github.com/coder/coder/pull/26016 fix",
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/26016" search:"fix"');
		expect(
			normalizeChatSearchInput('archived:true title:"chat title" fix'),
		).toBe('archived:true search:"chat title fix"');
	});

	it("combines duplicate title filters into one search filter", () => {
		expect(normalizeChatSearchInput("title:Fix title:Race")).toBe(
			'search:"Fix Race"',
		);
		expect(
			normalizeChatSearchInput('has_unread:true title:"chat title" title:Race'),
		).toBe('has_unread:true search:"chat title Race"');
	});

	it("preserves quoted websearch phrases in bare text", () => {
		// A leading/trailing quote pair is passed through so websearch_to_tsquery
		// can interpret it as a quoted phrase.
		expect(normalizeChatSearchInput('"fix race condition"')).toBe(
			'search:"fix race condition"',
		);
		expect(normalizeChatSearchInput('Fix "auth" middleware')).toBe(
			'search:Fix "auth" middleware',
		);
	});

	it("preserves websearch operators alongside a quoted phrase", () => {
		expect(normalizeChatSearchInput('"fix race" OR deadlock -timeout')).toBe(
			'search:"fix race" OR deadlock -timeout',
		);
	});

	it("strips stray quotes from bare text before wrapping", () => {
		// Unbalanced quotes would break the backend's query parser, which has no
		// escape handling for embedded quotes.
		expect(normalizeChatSearchInput("it's a \"test")).toBe(
			'search:"it\'s a test"',
		);
	});

	it("treats a trailing-colon filter as bare search text", () => {
		// `title:` is not a well-formed key:value pair, so it is wrapped as an
		// FTS phrase.
		expect(normalizeChatSearchInput("title:")).toBe('search:"title:"');
	});
});
