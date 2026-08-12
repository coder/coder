import { describe, expect, it } from "vitest";
import { buildChatSearchQuery, extractTypedFilters } from "./searchQuery";

describe("buildChatSearchQuery", () => {
	it("returns no query for empty input", () => {
		expect(buildChatSearchQuery([], "")).toBe(undefined);
		expect(buildChatSearchQuery([], "   ")).toBe(undefined);
	});

	it("wraps free text in one FTS token", () => {
		expect(buildChatSearchQuery([], "Fix")).toBe('search:"Fix"');
		expect(buildChatSearchQuery([], "fix auth middleware")).toBe(
			'search:"fix auth middleware"',
		);
		expect(buildChatSearchQuery([], "fix:lint")).toBe('search:"fix:lint"');
		expect(buildChatSearchQuery([], "http://example.com")).toBe(
			'search:"http://example.com"',
		);
	});

	it("combines structured filters with free text", () => {
		expect(
			buildChatSearchQuery([{ key: "has_unread", value: "true" }], "fix auth"),
		).toBe('has_unread:true search:"fix auth"');
	});

	it("normalizes structured filter values", () => {
		expect(
			buildChatSearchQuery([{ key: "pr_status", value: "open merged" }], ""),
		).toBe("pr_status:open,merged");
		for (const value of [
			"open, merged",
			"open merged",
			"open,merged",
			"  open  ,  merged  ",
			",,open,,,   merged,,",
		]) {
			expect(buildChatSearchQuery([{ key: "pr_status", value }], "")).toBe(
				"pr_status:open,merged",
			);
		}
		expect(
			buildChatSearchQuery([{ key: "pr_status", value: ",,  ," }], ""),
		).toBe(undefined);
		expect(
			buildChatSearchQuery(
				[
					{
						key: "diff_url",
						value: "github.com/coder/coder/pull/26016",
					},
				],
				"",
			),
		).toBe('diff_url:"https://github.com/coder/coder/pull/26016"');
	});

	it("emits no-lexeme text without marking it searchable", () => {
		for (const input of ["???", "___", ":-)", "!!!"]) {
			expect(buildChatSearchQuery([], input)).toBe(`search:"${input}"`);
		}
		expect(buildChatSearchQuery([], '"')).toBe('search:" "');
		// OR/AND/NOT are lexemes under the simple config (operators only between
		// operands), so a lone operator word is searchable in any casing.
		for (const input of ["or", "OR", "Or", "AND", "NOT"]) {
			expect(buildChatSearchQuery([], input)).toBe(`search:"${input}"`);
		}

		expect(
			buildChatSearchQuery([{ key: "has_unread", value: "true" }], "???"),
		).toBe('has_unread:true search:"???"');
	});

	it("emits Unicode letters as searchable text", () => {
		expect(buildChatSearchQuery([], "日本語")).toBe('search:"日本語"');
	});

	it("does not emit invalid structured filters", () => {
		for (const filter of [
			{ key: "pr_status", value: "banana" },
			{ key: "has_unread", value: "maybe" },
			{ key: "archived", value: "no" },
			{ key: "diff_url", value: "ftp://example.com/x" },
		]) {
			expect(buildChatSearchQuery([filter], "")).toBe(undefined);
		}
	});

	it("skips filters whose sanitized value is empty", () => {
		for (const key of ["pr_status", "diff_url"]) {
			for (const value of ['"', '""']) {
				expect(buildChatSearchQuery([{ key, value }], "")).toBe(undefined);
			}
		}
	});

	it("strips embedded quotes and trims before wrapping", () => {
		expect(buildChatSearchQuery([], '  Fix "auth" middleware  ')).toBe(
			'search:"Fix auth middleware"',
		);
	});

	it("preserves OR and negation while flattening quoted phrases", () => {
		expect(buildChatSearchQuery([], '"fix race" OR deadlock -timeout')).toBe(
			'search:"fix race OR deadlock -timeout"',
		);
	});

	it("never parses free text as structured filters", () => {
		for (const text of ["title:auth", "search:fix", "pr:12", "foo:bar"]) {
			expect(buildChatSearchQuery([], text)).toBe(`search:"${text}"`);
		}
	});
});

describe("extractTypedFilters", () => {
	it("extracts leading, middle, and trailing filters", () => {
		expect(extractTypedFilters("has_unread:true fix", [])).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix",
			consumed: true,
		});
		expect(extractTypedFilters("fix has_unread:true auth", [])).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix auth",
			consumed: true,
		});
		expect(extractTypedFilters("fix has_unread:true", [])).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix",
			consumed: true,
		});
	});

	it("returns multiple recognized filters", () => {
		expect(extractTypedFilters("has_unread:true archived:false", [])).toEqual({
			filters: [
				{ key: "has_unread", value: "true" },
				{ key: "archived", value: "false" },
			],
			remainingText: "",
			consumed: true,
		});
	});

	it("extracts complete quoted multi-word values", () => {
		expect(extractTypedFilters('pr_status:"open merged"', [])).toEqual({
			filters: [{ key: "pr_status", value: "open,merged" }],
			remainingText: "",
			consumed: true,
		});
	});

	it("merges whitespace-separated PR status continuations", () => {
		expect(extractTypedFilters("pr_status:open, merged", [])).toEqual({
			filters: [{ key: "pr_status", value: "open,merged" }],
			remainingText: "",
			consumed: true,
		});
		expect(extractTypedFilters("pr_status:open,merged", [])).toEqual({
			filters: [{ key: "pr_status", value: "open,merged" }],
			remainingText: "",
			consumed: true,
		});
		expect(extractTypedFilters("pr_status:open, merged, closed", [])).toEqual({
			filters: [{ key: "pr_status", value: "open,merged,closed" }],
			remainingText: "",
			consumed: true,
		});
	});

	it("leaves invalid or incomplete PR status continuations as text", () => {
		expect(extractTypedFilters("pr_status:open, bogus", [])).toEqual({
			filters: [],
			remainingText: "pr_status:open, bogus",
			consumed: false,
		});
		expect(extractTypedFilters("pr_status:open,", [])).toEqual({
			filters: [],
			remainingText: "pr_status:open,",
			consumed: false,
		});
	});

	it("does not consume an unbalanced quoted value", () => {
		expect(extractTypedFilters('pr_status:"open', [])).toEqual({
			filters: [],
			remainingText: 'pr_status:"open',
			consumed: false,
		});
	});

	it("returns active key replacements", () => {
		expect(
			extractTypedFilters("has_unread:false", [
				{ key: "has_unread", value: "true" },
			]),
		).toEqual({
			filters: [{ key: "has_unread", value: "false" }],
			remainingText: "",
			consumed: true,
		});
	});

	it("can consume an unchanged active value without returning a replacement", () => {
		expect(
			extractTypedFilters("has_unread:true", [
				{ key: "has_unread", value: "true" },
			]),
		).toEqual({
			filters: [],
			remainingText: "",
			consumed: true,
		});
	});

	it("uses the last value for duplicate keys in the same input", () => {
		expect(extractTypedFilters("has_unread:true has_unread:false", [])).toEqual(
			{
				filters: [{ key: "has_unread", value: "false" }],
				remainingText: "",
				consumed: true,
			},
		);
	});

	it("leaves unknown, incomplete, empty, and invalid filter-like text unchanged", () => {
		for (const text of [
			"foo:bar",
			"title:",
			"title:auth",
			"search:fix",
			"pr:12",
			"has_unread:",
			"has_unread:maybe",
			"archived:no",
			"pr_status:banana",
			"pr_status:,,",
			'diff_url:"ftp://example.com/x"',
			'pr_status:""',
			"http://example.com",
			"fix:lint",
		]) {
			expect(extractTypedFilters(text, [])).toEqual({
				filters: [],
				remainingText: text,
				consumed: false,
			});
		}
	});

	it("keeps invalid recognized filters as literal search text", () => {
		const extracted = extractTypedFilters("pr_status:banana", []);
		expect(buildChatSearchQuery([], extracted.remainingText)).toBe(
			'search:"pr_status:banana"',
		);
	});

	it("keeps everything after the first colon in diff URLs", () => {
		expect(
			extractTypedFilters("diff_url:https://github.com/coder/coder/pull/1", []),
		).toEqual({
			filters: [
				{
					key: "diff_url",
					value: "https://github.com/coder/coder/pull/1",
				},
			],
			remainingText: "",
			consumed: true,
		});
	});

	it("normalizes recognized key casing", () => {
		expect(extractTypedFilters("Has_Unread:true", [])).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "",
			consumed: true,
		});
	});
});
