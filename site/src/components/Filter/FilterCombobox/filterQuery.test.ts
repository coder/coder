import { describe, expect, it } from "vitest";
import {
	chipsToValues,
	composeFilterQuery,
	extractFreeText,
	filterValuesToChips,
	matchFacets,
	parseChipToken,
	parseTypedFacetPrefix,
	stringifyChipValues,
} from "./filterQuery";

const CHIP_KEYS = ["owner", "status", "template"] as const;

describe("filterQuery", () => {
	it("round-trips chip values through the query string", () => {
		const values = { owner: "me", status: "running", template: undefined };
		const query = composeFilterQuery(values, CHIP_KEYS, "devbox");
		expect(query).toBe("owner:me status:running devbox");
		expect(extractFreeText(query)).toBe("devbox");
		expect(filterValuesToChips(values, CHIP_KEYS)).toEqual([
			"owner:me",
			"status:running",
		]);
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

	it("keeps one value per facet when converting chips", () => {
		expect(
			chipsToValues(["owner:me", "owner:alice", "status:running"], CHIP_KEYS),
		).toEqual({
			owner: "alice",
			status: "running",
			template: undefined,
		});
	});

	it("matches typed facet prefixes by id, label, and alias", () => {
		const facets = [
			{ id: "owner" as const, label: "Owner", aliases: ["user"] },
			{ id: "status" as const, label: "Status" },
		];

		expect(parseTypedFacetPrefix("owner:me", facets)).toEqual({
			facetId: "owner",
			query: "me",
			freeText: "",
		});
		expect(parseTypedFacetPrefix("user:", facets)).toEqual({
			facetId: "owner",
			query: "",
			freeText: "",
		});
		expect(parseTypedFacetPrefix("Status:running", facets)).toEqual({
			facetId: "status",
			query: "running",
			freeText: "",
		});
		expect(parseTypedFacetPrefix("nope:", facets)).toBeNull();
	});

	it("matches a facet typed after free-text name search", () => {
		const facets = [
			{ id: "owner" as const, label: "Owner", aliases: ["user"] },
		];

		expect(parseTypedFacetPrefix("pink owner:", facets)).toEqual({
			facetId: "owner",
			query: "",
			freeText: "pink",
		});
		expect(
			parseTypedFacetPrefix("pink-mockingbird-23 user:al", facets),
		).toEqual({
			facetId: "owner",
			query: "al",
			freeText: "pink-mockingbird-23",
		});
	});

	it("matches category typeahead prefixes on id, label, and alias", () => {
		const facets = [
			{ id: "owner" as const, label: "Owner", aliases: ["user"] },
			{ id: "status" as const, label: "Status" },
			{ id: "template" as const, label: "Template" },
			{ id: "organization" as const, label: "Organization" },
		];

		expect(matchFacets("ow", facets).map((facet) => facet.id)).toEqual([
			"owner",
		]);
		expect(matchFacets("stat", facets).map((facet) => facet.id)).toEqual([
			"status",
		]);
		expect(matchFacets("user", facets).map((facet) => facet.id)).toEqual([
			"owner",
		]);
		expect(matchFacets("o", facets).map((facet) => facet.id)).toEqual([
			"owner",
			"organization",
		]);
		expect(matchFacets("do", facets)).toEqual([]);
		expect(matchFacets("  ", facets)).toEqual([]);
	});
});
