import { describe, expect, it } from "vitest";
import {
	buildChatSearchQuery,
	CHAT_SEARCH_KNOWN_FILTER_KEYS,
	extractTypedFilters,
} from "./searchQuery";

const knownKeys = CHAT_SEARCH_KNOWN_FILTER_KEYS;

describe("buildChatSearchQuery", () => {
	it("returns no query for empty input", () => {
		expect(buildChatSearchQuery([], "")).toEqual({
			query: undefined,
			hasSearchText: false,
		});
		expect(buildChatSearchQuery([], "   ")).toEqual({
			query: undefined,
			hasSearchText: false,
		});
	});

	it("wraps free text in one FTS token", () => {
		expect(buildChatSearchQuery([], "Fix")).toEqual({
			query: 'search:"Fix"',
			hasSearchText: true,
		});
		expect(buildChatSearchQuery([], "fix auth middleware")).toEqual({
			query: 'search:"fix auth middleware"',
			hasSearchText: true,
		});
		expect(buildChatSearchQuery([], "fix:lint")).toEqual({
			query: 'search:"fix:lint"',
			hasSearchText: true,
		});
		expect(buildChatSearchQuery([], "http://example.com")).toEqual({
			query: 'search:"http://example.com"',
			hasSearchText: true,
		});
	});

	it("combines structured filters with free text", () => {
		expect(
			buildChatSearchQuery([{ key: "has_unread", value: "true" }], "fix auth"),
		).toEqual({
			query: 'has_unread:true search:"fix auth"',
			hasSearchText: true,
		});
	});

	it("normalizes structured filter values", () => {
		expect(
			buildChatSearchQuery([{ key: "pr_status", value: "open merged" }], ""),
		).toEqual({
			query: "pr_status:open,merged",
			hasSearchText: false,
		});
		for (const value of [
			"open, merged",
			"open merged",
			"open,merged",
			"  open  ,  merged  ",
			",,open,,,   merged,,",
		]) {
			expect(buildChatSearchQuery([{ key: "pr_status", value }], "")).toEqual({
				query: "pr_status:open,merged",
				hasSearchText: false,
			});
		}
		expect(
			buildChatSearchQuery([{ key: "pr_status", value: ",,  ," }], ""),
		).toEqual({
			query: undefined,
			hasSearchText: false,
		});
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
		).toEqual({
			query: 'diff_url:"https://github.com/coder/coder/pull/26016"',
			hasSearchText: false,
		});
	});

	it("emits no-lexeme text without marking it searchable", () => {
		for (const input of ["???", "___", ":-)", "!!!"]) {
			expect(buildChatSearchQuery([], input)).toEqual({
				query: `search:"${input}"`,
				hasSearchText: false,
			});
		}
		expect(buildChatSearchQuery([], '"')).toEqual({
			query: undefined,
			hasSearchText: false,
		});
		// OR/AND/NOT are lexemes under the simple config (operators only between
		// operands), so a lone operator word is searchable in any casing.
		for (const input of ["or", "OR", "Or", "AND", "NOT"]) {
			expect(buildChatSearchQuery([], input)).toEqual({
				query: `search:"${input}"`,
				hasSearchText: true,
			});
		}

		expect(
			buildChatSearchQuery([{ key: "has_unread", value: "true" }], "???"),
		).toEqual({
			query: 'has_unread:true search:"???"',
			hasSearchText: false,
		});
	});

	it("emits Unicode letters as searchable text", () => {
		expect(buildChatSearchQuery([], "日本語")).toEqual({
			query: 'search:"日本語"',
			hasSearchText: true,
		});
	});

	it("does not emit invalid structured filters", () => {
		for (const filter of [
			{ key: "pr_status", value: "banana" },
			{ key: "has_unread", value: "maybe" },
			{ key: "archived", value: "no" },
			{ key: "diff_url", value: "ftp://example.com/x" },
			{ key: "diff_url", value: "https:///pull/1" },
		]) {
			expect(buildChatSearchQuery([filter], "")).toEqual({
				query: undefined,
				hasSearchText: false,
			});
		}
	});

	it("skips filters whose sanitized value is empty", () => {
		for (const key of ["pr_status", "diff_url"]) {
			for (const value of ['"', '""']) {
				expect(buildChatSearchQuery([{ key, value }], "")).toEqual({
					query: undefined,
					hasSearchText: false,
				});
			}
		}
	});

	it("strips embedded quotes and trims before wrapping", () => {
		expect(buildChatSearchQuery([], '  Fix "auth" middleware  ')).toEqual({
			query: 'search:"Fix auth middleware"',
			hasSearchText: true,
		});
	});

	it("preserves OR and negation while flattening quoted phrases", () => {
		expect(buildChatSearchQuery([], '"fix race" OR deadlock -timeout')).toEqual(
			{
				query: 'search:"fix race OR deadlock -timeout"',
				hasSearchText: true,
			},
		);
	});

	it("never parses free text as structured filters", () => {
		for (const text of ["title:auth", "search:fix", "pr:12", "foo:bar"]) {
			expect(buildChatSearchQuery([], text)).toEqual({
				query: `search:"${text}"`,
				hasSearchText: true,
			});
		}
	});
});

describe("extractTypedFilters", () => {
	it("extracts leading, middle, and trailing filters", () => {
		expect(extractTypedFilters("has_unread:true fix", knownKeys, [])).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix",
			consumed: true,
		});
		expect(
			extractTypedFilters("fix has_unread:true auth", knownKeys, []),
		).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix auth",
			consumed: true,
		});
		expect(extractTypedFilters("fix has_unread:true", knownKeys, [])).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix",
			consumed: true,
		});
	});

	it("returns multiple recognized filters", () => {
		expect(
			extractTypedFilters("has_unread:true archived:false", knownKeys, []),
		).toEqual({
			filters: [
				{ key: "has_unread", value: "true" },
				{ key: "archived", value: "false" },
			],
			remainingText: "",
			consumed: true,
		});
	});

	it("extracts complete quoted multi-word values", () => {
		expect(
			extractTypedFilters('pr_status:"open merged"', knownKeys, []),
		).toEqual({
			filters: [{ key: "pr_status", value: "open merged" }],
			remainingText: "",
			consumed: true,
		});
	});

	it("does not consume an unbalanced quoted value", () => {
		expect(extractTypedFilters('pr_status:"open', knownKeys, [])).toEqual({
			filters: [],
			remainingText: 'pr_status:"open',
			consumed: false,
		});
	});

	it("returns active key replacements", () => {
		expect(
			extractTypedFilters("has_unread:false", knownKeys, [
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
			extractTypedFilters("has_unread:true", knownKeys, [
				{ key: "has_unread", value: "true" },
			]),
		).toEqual({
			filters: [],
			remainingText: "",
			consumed: true,
		});
	});

	it("uses the last value for duplicate keys in the same input", () => {
		expect(
			extractTypedFilters("has_unread:true has_unread:false", knownKeys, []),
		).toEqual({
			filters: [{ key: "has_unread", value: "false" }],
			remainingText: "",
			consumed: true,
		});
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
			expect(extractTypedFilters(text, knownKeys, [])).toEqual({
				filters: [],
				remainingText: text,
				consumed: false,
			});
		}
	});

	it("keeps invalid recognized filters as literal search text", () => {
		const extracted = extractTypedFilters("pr_status:banana", knownKeys, []);
		expect(buildChatSearchQuery([], extracted.remainingText)).toEqual({
			query: 'search:"pr_status:banana"',
			hasSearchText: true,
		});
	});

	it("keeps everything after the first colon in diff URLs", () => {
		expect(
			extractTypedFilters(
				"diff_url:https://github.com/coder/coder/pull/1",
				knownKeys,
				[],
			),
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
		expect(extractTypedFilters("Has_Unread:true", knownKeys, [])).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "",
			consumed: true,
		});
	});
});
