import { describe, expect, it } from "vitest";
import { MockOpenPreviewResult } from "#/testHelpers/chatEntities";
import { parseOpenPreviewResult } from "./PreviewPanelContext";

describe("parseOpenPreviewResult", () => {
	it("opens a url_path of / at the root", () => {
		expect(
			parseOpenPreviewResult({ ...MockOpenPreviewResult, url_path: "/" }),
		).toEqual({
			title: MockOpenPreviewResult.title,
			workspaceId: MockOpenPreviewResult.workspace_id,
			agentName: MockOpenPreviewResult.agent_name,
			target: { kind: "port", port: MockOpenPreviewResult.port },
			urlPath: undefined,
		});
	});

	it.each([
		{ name: "an error", value: { error: "port must be set" } },
		{ name: "no target", value: { ...MockOpenPreviewResult, port: undefined } },
		{
			name: "two targets",
			value: { ...MockOpenPreviewResult, app_slug: "code-server" },
		},
		{
			name: "an out-of-range port",
			value: { ...MockOpenPreviewResult, port: 70000 },
		},
		{
			name: "a preview ID that could change the hostname",
			value: {
				...MockOpenPreviewResult,
				port: undefined,
				preview_id: "evil.example.com/",
			},
		},
	])("rejects $name", ({ value }) => {
		expect(parseOpenPreviewResult(value)).toBeUndefined();
	});
});
