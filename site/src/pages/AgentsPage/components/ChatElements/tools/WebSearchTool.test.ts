import { describe, expect, it } from "vitest";
import { getWebSearchToolData } from "./WebSearchTool";

describe("getWebSearchToolData", () => {
	it("reads OpenAI queries from the args and sources from the result", () => {
		expect(
			getWebSearchToolData(
				{ queries: [" tokyo population ", "tokyo census", "tokyo census", ""] },
				{
					sources: [
						{ url: "https://www.metro.tokyo.lg.jp/" },
						{ url: "http://example.com/tokyo" },
						{ url: "https://www.metro.tokyo.lg.jp/" },
					],
				},
			),
		).toEqual({
			queries: ["tokyo population", "tokyo census"],
			sourceUrls: [
				"https://www.metro.tokyo.lg.jp/",
				"http://example.com/tokyo",
			],
			searchFinished: true,
		});
	});

	it("drops source URLs that are not http or https", () => {
		expect(
			getWebSearchToolData(
				{ queries: [] },
				{
					sources: [
						{ url: "data:text/html,unsafe" },
						{ url: "ftp://example.com/file" },
						{ url: "not a url" },
						{ url: 42 },
						"https://example.com/not-a-record",
						{ url: "https://example.com/kept" },
					],
				},
			).sourceUrls,
		).toEqual(["https://example.com/kept"]);
	});

	it("marks an OpenAI search without queries as finished", () => {
		expect(getWebSearchToolData({ queries: [] }, undefined)).toEqual({
			queries: [],
			sourceUrls: [],
			searchFinished: true,
		});
	});

	it("reads queries that fixtures pass JSON-encoded", () => {
		expect(
			getWebSearchToolData({ queries: JSON.stringify(["coder agents"]) }, {}),
		).toEqual({
			queries: ["coder agents"],
			sourceUrls: [],
			searchFinished: true,
		});
	});

	it("reads the Anthropic query from the args without marking it finished", () => {
		expect(getWebSearchToolData({ query: "coder templates" }, {})).toEqual({
			queries: ["coder templates"],
			sourceUrls: [],
			searchFinished: false,
		});
	});

	it("returns no queries for calls persisted without args", () => {
		expect(getWebSearchToolData(undefined, {})).toEqual({
			queries: [],
			sourceUrls: [],
			searchFinished: false,
		});
	});
});
