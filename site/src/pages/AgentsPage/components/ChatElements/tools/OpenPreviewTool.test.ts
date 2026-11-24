import { describe, expect, it } from "vitest";
import { MockOpenPreviewResult } from "#/testHelpers/chatEntities";
import { parseOpenPreviewResult } from "./OpenPreviewTool";

describe("parseOpenPreviewResult", () => {
	it.each([
		{ name: "an error", value: { error: "port must be set" } },
		{
			name: "no target",
			value: { ...MockOpenPreviewResult, preview_slug: undefined },
		},
		{
			name: "a slug without the preview prefix",
			value: { ...MockOpenPreviewResult, preview_slug: "evil.example.com/" },
		},
		{ name: "an empty title", value: { ...MockOpenPreviewResult, title: "" } },
		{
			name: "a title over 80 characters",
			value: { ...MockOpenPreviewResult, title: "a".repeat(81) },
		},
		{
			name: "a title with a bidi override or another invisible character",
			value: { ...MockOpenPreviewResult, title: "\u202Etis\u200BscoD" },
		},
		{
			name: "a title with a newline",
			value: { ...MockOpenPreviewResult, title: "Docs\nsite" },
		},
		{
			name: "a title with a line separator",
			value: { ...MockOpenPreviewResult, title: "Docs\u2028site" },
		},
		{
			name: "a title with a paragraph separator",
			value: { ...MockOpenPreviewResult, title: "Docs\u2029site" },
		},
		{
			name: "a URL path without a leading slash",
			value: { ...MockOpenPreviewResult, url_path: "docs" },
		},
		{
			name: "a URL path that is not a string",
			value: { ...MockOpenPreviewResult, url_path: 2 },
		},
	])("rejects $name", ({ value }) => {
		expect(parseOpenPreviewResult(value)).toBeUndefined();
	});

	it.each([
		{
			name: "a file preview at a URL path",
			value: {
				...MockOpenPreviewResult,
				port: undefined,
				preview_slug: "preview--0123abcd",
				url_path: "/?page=2",
			},
			tab: { slug: "preview--0123abcd", urlPath: "/?page=2" },
		},
		{
			name: "a slug and URL path that only sign-in refuses",
			value: {
				...MockOpenPreviewResult,
				preview_slug: "preview--03000",
				url_path: "/docs#top",
			},
			tab: { slug: "preview--03000", urlPath: "/docs#top" },
		},
		{
			name: "a title of 80 characters but more UTF-16 code units",
			value: { ...MockOpenPreviewResult, title: "🚀".repeat(80) },
			tab: { label: "🚀".repeat(80) },
		},
	])("accepts $name", ({ value, tab }) => {
		expect(parseOpenPreviewResult(value)).toMatchObject(tab);
	});
});
