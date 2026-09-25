import { describe, expect, it } from "vitest";
import { getSourcePillDisplay } from "./WebSearchSources";
import {
	getWebSearchAction,
	getWebSearchLabel,
	getWebSearchState,
} from "./WebSearchTool";

describe("getWebSearchAction", () => {
	it("reads OpenAI search queries, trimmed and deduplicated", () => {
		expect(
			getWebSearchAction({
				type: "search",
				queries: [" tokyo population ", "tokyo census", "tokyo census", ""],
			}),
		).toEqual({
			type: "search",
			queries: ["tokyo population", "tokyo census"],
		});
	});

	it("reads queries that fixtures pass JSON-encoded", () => {
		expect(
			getWebSearchAction({ queries: JSON.stringify(["coder agents"]) }),
		).toEqual({ type: "search", queries: ["coder agents"] });
	});

	it("reads the Anthropic query", () => {
		expect(getWebSearchAction({ query: "coder templates" })).toEqual({
			type: "search",
			queries: ["coder templates"],
		});
	});

	it("returns a search without queries for empty or missing args", () => {
		expect(getWebSearchAction({ type: "search" })).toEqual({
			type: "search",
			queries: [],
		});
		expect(getWebSearchAction({})).toEqual({ type: "search", queries: [] });
		expect(getWebSearchAction(undefined)).toEqual({
			type: "search",
			queries: [],
		});
	});

	it("reads open_page and find_in_page actions", () => {
		expect(
			getWebSearchAction({ type: "open_page", url: "https://coder.com/docs" }),
		).toEqual({ type: "open_page", url: "https://coder.com/docs" });
		expect(
			getWebSearchAction({
				type: "find_in_page",
				url: "https://coder.com/docs",
				pattern: "templates",
			}),
		).toEqual({
			type: "find_in_page",
			url: "https://coder.com/docs",
			pattern: "templates",
		});
	});
});

describe("getWebSearchState", () => {
	it("is running while the message streams without a result", () => {
		expect(
			getWebSearchState({
				status: "running",
				result: undefined,
				isError: false,
			}),
		).toBe("running");
	});

	it("did not finish when the message ended without a result", () => {
		expect(
			getWebSearchState({
				status: "completed",
				result: undefined,
				isError: false,
			}),
		).toBe("unfinished");
	});

	it("failed when the result is an error", () => {
		expect(
			getWebSearchState({
				status: "error",
				result: { error: "web search failed" },
				isError: true,
			}),
		).toBe("failed");
	});

	it("finished when a result arrived, even without queries in the args", () => {
		expect(
			getWebSearchState({ status: "completed", result: {}, isError: false }),
		).toBe("finished");
	});
});

describe("getWebSearchLabel", () => {
	const search = getWebSearchAction({ queries: ["q1", "q2"] });
	const bareSearch = getWebSearchAction({ type: "search" });
	const openPage = getWebSearchAction({
		type: "open_page",
		url: "https://coder.com/docs",
	});
	const findInPage = getWebSearchAction({
		type: "find_in_page",
		url: "https://coder.com/docs",
		pattern: "templates",
	});

	it.each([
		[search, "running", "Searching for q1, q2"],
		[search, "finished", "Searched for q1, q2"],
		[search, "failed", "Failed to search for q1, q2"],
		[search, "unfinished", "Did not finish searching for q1, q2"],
		[bareSearch, "running", "Searching the web"],
		[bareSearch, "finished", "Searched the web"],
		[openPage, "running", "Opening https://coder.com/docs"],
		[openPage, "finished", "Opened https://coder.com/docs"],
		[openPage, "failed", "Failed to open https://coder.com/docs"],
		[openPage, "unfinished", "Did not finish opening https://coder.com/docs"],
		[findInPage, "running", "Searching https://coder.com/docs for templates"],
		[findInPage, "finished", "Searched https://coder.com/docs for templates"],
		[
			findInPage,
			"failed",
			"Failed to search https://coder.com/docs for templates",
		],
		[
			findInPage,
			"unfinished",
			"Did not finish searching https://coder.com/docs for templates",
		],
	] as const)("labels %o as %s: %s", (action, state, label) => {
		expect(getWebSearchLabel(action, state)).toBe(label);
	});
});

describe("getSourcePillDisplay", () => {
	it("links http and https URLs", () => {
		expect(
			getSourcePillDisplay({ url: "https://coder.com/changelog", title: "" })
				.href,
		).toBe("https://coder.com/changelog");
		expect(
			getSourcePillDisplay({ url: "http://example.com/", title: "" }).href,
		).toBe("http://example.com/");
	});

	it.each([
		"ftp://example.com/release.txt",
		"data:text/html,unsafe",
		"mailto:team@example.com",
		"not a url",
	])("does not link %s", (url) => {
		expect(getSourcePillDisplay({ url, title: "" }).href).toBeUndefined();
	});

	it("labels a page by title, else hostname plus path, else the raw URL", () => {
		expect(
			getSourcePillDisplay({ url: "https://coder.com/docs", title: "Docs" })
				.label,
		).toBe("Docs");
		expect(
			getSourcePillDisplay({ url: "https://coder.com/docs/ai", title: "" })
				.label,
		).toBe("coder.com/docs/ai");
		expect(
			getSourcePillDisplay({ url: "https://coder.com/", title: "" }).label,
		).toBe("coder.com");
		expect(getSourcePillDisplay({ url: "not a url", title: "" }).label).toBe(
			"not a url",
		);
	});
});
