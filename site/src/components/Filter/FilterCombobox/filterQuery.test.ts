import { describe, expect, it } from "vitest";
import {
	collectValueSuggestions,
	composeFilterQuery,
	dedupeChips,
	extractFreeText,
	matchCategories,
	parseChipToken,
	parseTypedCategoryPrefix,
	queryToChips,
} from "./filterQuery";

const CHIP_KEYS = ["owner", "status", "template"] as const;

describe("filterQuery", () => {
	it("round-trips chip tokens through the query string", () => {
		const tokens = ["owner:me", "status:running"];
		const query = composeFilterQuery(tokens, CHIP_KEYS, "devbox");
		expect(query).toBe("owner:me status:running devbox");
		expect(extractFreeText(query, CHIP_KEYS)).toBe("devbox");
		expect(queryToChips(query, CHIP_KEYS)).toEqual([
			"owner:me",
			"status:running",
		]);
	});

	it("preserves the order chips were added when composing", () => {
		// Insertion order must survive, not the CHIP_KEYS category order
		// (owner, status, template).
		expect(
			composeFilterQuery(
				["template:docker", "owner:me", "status:running"],
				CHIP_KEYS,
				"",
			),
		).toBe("template:docker owner:me status:running");
	});

	it("appends a newly added chip to the end of the query", () => {
		const existing = ["owner:me", "status:running"];
		expect(
			composeFilterQuery([...existing, "template:docker"], CHIP_KEYS, ""),
		).toBe("owner:me status:running template:docker");
	});

	it("preserves chip appearance order when parsing a query", () => {
		expect(
			queryToChips("template:docker owner:me status:running", CHIP_KEYS),
		).toEqual(["template:docker", "owner:me", "status:running"]);
	});

	it("preserves unrecognized key:value tokens as free text", () => {
		// Documented backend filters that are not chip categories must survive.
		expect(
			extractFreeText("owner:me dormant:true outdated:true", CHIP_KEYS),
		).toBe("dormant:true outdated:true");
		// Hyphenated keys must not be corrupted into a bare "has-" search.
		expect(extractFreeText("owner:me has-agent:connected", CHIP_KEYS)).toBe(
			"has-agent:connected",
		);
		// Quoted values are kept intact.
		expect(extractFreeText('name:"my box" owner:me', CHIP_KEYS)).toBe(
			'name:"my box"',
		);
	});

	it("round-trips a query mixing chips and backend-only filters", () => {
		const query = "owner:me dormant:true";
		const chips = queryToChips(query, CHIP_KEYS);
		const freeText = extractFreeText(query, CHIP_KEYS);
		expect(composeFilterQuery(chips, CHIP_KEYS, freeText)).toBe(
			"owner:me dormant:true",
		);
	});

	it("quotes chip values that contain spaces", () => {
		expect(composeFilterQuery(["template:my template"], CHIP_KEYS, "")).toBe(
			'template:"my template"',
		);
	});

	it("parses chip tokens and rejects unknown keys", () => {
		expect(parseChipToken("owner:alice", CHIP_KEYS)).toEqual({
			key: "owner",
			value: "alice",
		});
		expect(parseChipToken("unknown:x", CHIP_KEYS)).toBeNull();
	});

	it("parses chip tokens case-insensitively", () => {
		expect(parseChipToken("Owner:alice", CHIP_KEYS)).toEqual({
			key: "owner",
			value: "alice",
		});
		expect(queryToChips("Owner:me STATUS:running", CHIP_KEYS)).toEqual([
			"owner:me",
			"status:running",
		]);
	});

	it("keeps one value per category, first position and last value win", () => {
		// A repeated category keeps its first-seen position but takes the
		// latest value provided for it.
		expect(
			dedupeChips(["owner:me", "status:running", "owner:alice"], CHIP_KEYS),
		).toEqual(["owner:alice", "status:running"]);
	});

	it("matches typed category prefixes by key, label, and alias", () => {
		const categories = [
			{ key: "owner", label: "Owner", aliases: ["user"] },
			{ key: "status", label: "Status" },
		];

		expect(parseTypedCategoryPrefix("owner:me", categories)).toEqual({
			categoryKey: "owner",
			query: "me",
			freeText: "",
		});
		expect(parseTypedCategoryPrefix("user:", categories)).toEqual({
			categoryKey: "owner",
			query: "",
			freeText: "",
		});
		expect(parseTypedCategoryPrefix("Status:running", categories)).toEqual({
			categoryKey: "status",
			query: "running",
			freeText: "",
		});
		expect(parseTypedCategoryPrefix("nope:", categories)).toBeNull();
	});

	it("resolves the last key: fragment when an earlier one is not a category", () => {
		// `has-agent:connected` is a backend-only filter, not a category, so the
		// scan must fall through to the trailing `owner:` prefix.
		expect(
			parseTypedCategoryPrefix("has-agent:connected owner:al", [
				{ key: "owner", label: "Owner" },
			]),
		).toEqual({
			categoryKey: "owner",
			query: "al",
			freeText: "has-agent:connected",
		});
	});

	it("matches a category typed after free-text name search", () => {
		const categories = [{ key: "owner", label: "Owner", aliases: ["user"] }];

		expect(parseTypedCategoryPrefix("pink owner:", categories)).toEqual({
			categoryKey: "owner",
			query: "",
			freeText: "pink",
		});
		expect(
			parseTypedCategoryPrefix("pink-mockingbird-23 user:al", categories),
		).toEqual({
			categoryKey: "owner",
			query: "al",
			freeText: "pink-mockingbird-23",
		});
	});

	it("matches category typeahead prefixes on key, label, and alias", () => {
		const categories = [
			{ key: "owner", label: "Owner", aliases: ["user"] },
			{ key: "status", label: "Status" },
			{ key: "template", label: "Template" },
			{ key: "organization", label: "Organization" },
		];

		expect(matchCategories("ow", categories).map((c) => c.key)).toEqual([
			"owner",
		]);
		expect(matchCategories("stat", categories).map((c) => c.key)).toEqual([
			"status",
		]);
		expect(matchCategories("user", categories).map((c) => c.key)).toEqual([
			"owner",
		]);
		expect(matchCategories("o", categories).map((c) => c.key)).toEqual([
			"owner",
			"organization",
		]);
		expect(matchCategories("do", categories)).toEqual([]);
		expect(matchCategories("  ", categories)).toEqual([]);
	});

	it("collects cross-category value suggestions", () => {
		const categories = [
			{ key: "owner", label: "Owner" },
			{ key: "status", label: "Status" },
		];
		const optionsByKey = new Map([
			[
				"owner",
				[
					{ label: "testuser01", value: "testuser01" },
					{ label: "alice", value: "alice" },
				],
			],
			[
				"status",
				[
					{ label: "Running", value: "running" },
					{ label: "Stopped", value: "stopped" },
				],
			],
		]);

		expect(
			collectValueSuggestions("test", categories, optionsByKey, []).map(
				(suggestion) => suggestion.token,
			),
		).toEqual(["owner:testuser01"]);
		expect(
			collectValueSuggestions("stop", categories, optionsByKey, []).map(
				(suggestion) =>
					`${suggestion.categoryLabel}: ${suggestion.option.label}`,
			),
		).toEqual(["Status: Stopped"]);
		expect(
			collectValueSuggestions("test", categories, optionsByKey, [
				"owner:testuser01",
			]),
		).toEqual([]);
	});

	it("uses an option's explicit token when suggesting values", () => {
		const categories = [{ key: "attributes", label: "Attributes" }];
		const optionsByKey = new Map([
			[
				"attributes",
				[
					{ label: "Outdated", value: "outdated", token: "outdated:true" },
					{ label: "Dormant", value: "dormant", token: "dormant:true" },
				],
			],
		]);

		expect(
			collectValueSuggestions("out", categories, optionsByKey, []).map(
				(suggestion) => suggestion.token,
			),
		).toEqual(["outdated:true"]);
		// An already-applied attribute chip is filtered out by its token.
		expect(
			collectValueSuggestions("dormant", categories, optionsByKey, [
				"dormant:true",
			]),
		).toEqual([]);
	});
});
