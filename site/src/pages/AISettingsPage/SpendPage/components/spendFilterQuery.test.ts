import { describe, expect, it } from "vitest";
import { queryToSpendFilter, spendFilterToQuery } from "./spendFilterQuery";

describe("spendFilterToQuery", () => {
	it("emits one chip per set dimension plus trailing free text", () => {
		expect(
			spendFilterToQuery({ provider_name: "openai", model: "gpt-4o" }, "alice"),
		).toBe("provider_name:openai model:gpt-4o alice");
	});

	it("quotes values that contain spaces", () => {
		expect(spendFilterToQuery({ client: "Claude Code" }, "")).toBe(
			'client:"Claude Code"',
		);
	});

	it("omits empty dimensions and search", () => {
		expect(spendFilterToQuery({}, "")).toBe("");
		expect(spendFilterToQuery({ provider_name: "" }, "  ")).toBe("");
	});
});

describe("queryToSpendFilter", () => {
	it("parses chips into dimensions and the rest into search", () => {
		expect(
			queryToSpendFilter('provider_name:openai client:"Claude Code" alice'),
		).toEqual({
			dimensions: { provider_name: "openai", client: "Claude Code" },
			search: "alice",
		});
	});

	it("returns empty state for a blank query", () => {
		expect(queryToSpendFilter("")).toEqual({ dimensions: {}, search: "" });
	});

	it("keeps unknown key:value tokens as free text", () => {
		expect(queryToSpendFilter("initiator:me alice")).toEqual({
			dimensions: {},
			search: "initiator:me alice",
		});
	});
});

describe("spend filter round-trip", () => {
	it.each([
		{ dimensions: {}, search: "" },
		{ dimensions: { provider_name: "openai" }, search: "" },
		{ dimensions: { client: "Claude Code" }, search: "bob" },
		{
			dimensions: {
				provider_name: "anthropic-main",
				client: "Claude Code",
				model: "claude-opus-4-6",
			},
			search: "team lead",
		},
	])("survives $dimensions / $search", ({ dimensions, search }) => {
		const query = spendFilterToQuery(dimensions, search);
		expect(queryToSpendFilter(query)).toEqual({ dimensions, search });
	});
});
