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

	it("converts bare search text into a title filter", () => {
		expect(normalizeChatSearchInput("Fix")).toBe('title:"Fix"');
		expect(normalizeChatSearchInput("fix auth middleware")).toBe(
			'title:"fix auth middleware"',
		);
		expect(normalizeChatSearchInput("fix:lint")).toBe('title:"fix:lint"');
	});

	it("combines key:value filters with a title fallback for bare text", () => {
		expect(normalizeChatSearchInput("has_unread:true fix auth")).toBe(
			'has_unread:true title:"fix auth"',
		);
		expect(normalizeChatSearchInput("archived:true fix:lint")).toBe(
			'archived:true title:"fix:lint"',
		);
		expect(normalizeChatSearchInput("fix has_unread:true auth")).toBe(
			'has_unread:true title:"fix auth"',
		);
		expect(
			normalizeChatSearchInput(
				"diff_url:https://github.com/coder/coder/pull/26016 fix",
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/26016" title:"fix"');
		expect(
			normalizeChatSearchInput('archived:true title:"chat title" fix'),
		).toBe('archived:true title:"chat title fix"');
	});

	it("combines duplicate title filters into one title filter", () => {
		expect(normalizeChatSearchInput("title:Fix title:Race")).toBe(
			'title:"Fix Race"',
		);
		expect(
			normalizeChatSearchInput('has_unread:true title:"chat title" title:Race'),
		).toBe('has_unread:true title:"chat title Race"');
	});

	it("strips quotes from bare text", () => {
		expect(normalizeChatSearchInput('Fix "auth" middleware')).toBe(
			'title:"Fix auth middleware"',
		);
	});

	it("treats a trailing-colon filter as bare title text", () => {
		// `title:` is not a well-formed key:value pair, so it should be searched
		// for as a literal title substring.
		expect(normalizeChatSearchInput("title:")).toBe('title:"title:"');
	});
});
