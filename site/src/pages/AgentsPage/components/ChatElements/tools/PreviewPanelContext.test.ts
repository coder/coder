import { describe, expect, it } from "vitest";
import { MockOpenPreviewResult } from "#/testHelpers/chatEntities";
import { parseOpenPreviewResult } from "./PreviewPanelContext";

describe("parseOpenPreviewResult", () => {
	it.each([
		{ name: "an error", value: { error: "port must be set" } },
		{ name: "no target", value: { ...MockOpenPreviewResult, port: undefined } },
		{
			name: "an unknown kind",
			value: { ...MockOpenPreviewResult, kind: "tunnel" },
		},
		{
			name: "an out-of-range port",
			value: { ...MockOpenPreviewResult, port: 70000 },
		},
		{
			name: "a preview slug that could change the hostname",
			value: {
				...MockOpenPreviewResult,
				kind: "file",
				port: undefined,
				preview_slug: "evil.example.com/",
			},
		},
	])("rejects $name", ({ value }) => {
		expect(parseOpenPreviewResult(value)).toBeUndefined();
	});
});
