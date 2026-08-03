import { describe, expect, it } from "vitest";
import {
	chipsToValues,
	collectValueSuggestions,
	composeFilterQuery,
	extractFreeText,
	filterValuesToChips,
	matchCategories,
	parseChipToken,
	parseTypedCategoryPrefix,
	queryToChips,
	stringifyChipValues,
} from "./filterQuery";

const CHIP_KEYS = ["owner", "status", "template"] as const;

describe("filterQuery", () => {
	it("round-trips chip values through the query string", () => {
		const values = { owner: "me", status: "running", template: undefined };
		const query = composeFilterQuery(values, CHIP_KEYS, "devbox");
		expect(query).toBe("owner:me status:running devbox");
		expect(extractFreeText(query, CHIP_KEYS)).toBe("devbox");
		expect(filterValuesToChips(values, CHIP_KEYS)).toEqual([
			"owner:me",
			"status:running",
		]);
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
		expect(
			composeFilterQuery(chipsToValues(chips, CHIP_KEYS), CHIP_KEYS, freeText),
		).toBe("owner:me dormant:true");
	});

	it("quotes chip values that contain spaces", () => {
		expect(stringifyChipValues({ template: "my template" }, ["template"])).toBe(
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

	it("keeps one value per category when converting chips", () => {
		expect(
			chipsToValues(["owner:me", "owner:alice", "status:running"], CHIP_KEYS),
		).toEqual({
			owner: "alice",
			status: "running",
			template: undefined,
		});
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
});
