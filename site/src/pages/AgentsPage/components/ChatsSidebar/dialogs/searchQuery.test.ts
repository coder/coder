import { describe, expect, it } from "vitest";
import { buildChatSearchQuery, extractTypedFilters } from "./searchQuery";

const knownKeys = new Set(["archived", "diff_url", "has_unread", "pr_status"]);

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
			query: 'pr_status:"open merged"',
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

	it("does not emit punctuation-only free text", () => {
		for (const input of ['"', "???", "___", ":-)", "!!!"]) {
			expect(buildChatSearchQuery([], input)).toEqual({
				query: undefined,
				hasSearchText: false,
			});
		}

		expect(
			buildChatSearchQuery([{ key: "has_unread", value: "true" }], "???"),
		).toEqual({
			query: "has_unread:true",
			hasSearchText: false,
		});
	});

	it("emits Unicode letters as searchable text", () => {
		expect(buildChatSearchQuery([], "日本語")).toEqual({
			query: 'search:"日本語"',
			hasSearchText: true,
		});
	});

	it("skips filters whose sanitized value is empty", () => {
		for (const value of ['"', '""']) {
			expect(buildChatSearchQuery([{ key: "pr_status", value }], "")).toEqual({
				query: undefined,
				hasSearchText: false,
			});
		}
	});

	it("strips embedded quotes and trims before wrapping", () => {
		expect(buildChatSearchQuery([], '  Fix "auth" middleware  ')).toEqual({
			query: 'search:"Fix auth middleware"',
			hasSearchText: true,
		});
	});

	it("preserves websearch operators for backend FTS parsing", () => {
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
		expect(
			extractTypedFilters("has_unread:true fix", knownKeys, new Set()),
		).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix",
			consumed: true,
		});
		expect(
			extractTypedFilters("fix has_unread:true auth", knownKeys, new Set()),
		).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix auth",
			consumed: true,
		});
		expect(
			extractTypedFilters("fix has_unread:true", knownKeys, new Set()),
		).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "fix ",
			consumed: true,
		});
	});

	it("extracts complete quoted multi-word values", () => {
		expect(
			extractTypedFilters('pr_status:"open merged"', knownKeys, new Set()),
		).toEqual({
			filters: [{ key: "pr_status", value: "open merged" }],
			remainingText: "",
			consumed: true,
		});
	});

	it("does not consume an unbalanced quoted value", () => {
		expect(
			extractTypedFilters('pr_status:"open', knownKeys, new Set()),
		).toEqual({
			filters: [],
			remainingText: 'pr_status:"open',
			consumed: false,
		});
	});

	it("consumes active duplicate keys without adding another filter", () => {
		expect(
			extractTypedFilters(
				"has_unread:false",
				knownKeys,
				new Set(["has_unread"]),
			),
		).toEqual({
			filters: [],
			remainingText: "",
			consumed: true,
		});
	});

	it("drops duplicate keys from the same input", () => {
		expect(
			extractTypedFilters(
				"has_unread:true has_unread:false",
				knownKeys,
				new Set(),
			),
		).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "",
			consumed: true,
		});
	});

	it("leaves unknown and incomplete filter-like text unchanged", () => {
		for (const text of [
			"foo:bar",
			"title:",
			"title:auth",
			"search:fix",
			"pr:12",
			"has_unread:",
			"http://example.com",
			"fix:lint",
		]) {
			expect(extractTypedFilters(text, knownKeys, new Set())).toEqual({
				filters: [],
				remainingText: text,
				consumed: false,
			});
		}
	});

	it("normalizes recognized key casing", () => {
		expect(
			extractTypedFilters("Has_Unread:true", knownKeys, new Set()),
		).toEqual({
			filters: [{ key: "has_unread", value: "true" }],
			remainingText: "",
			consumed: true,
		});
	});
});
