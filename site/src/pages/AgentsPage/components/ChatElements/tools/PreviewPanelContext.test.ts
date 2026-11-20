import { describe, expect, it, vi } from "vitest";
import { MockOpenPreviewResult } from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import {
	getPreviewPanelContextValue,
	parseOpenPreviewResult,
} from "./PreviewPanelContext";

describe("parseOpenPreviewResult", () => {
	it.each([
		{ name: "an error", value: { error: "port must be set" } },
		{
			name: "no target",
			value: { ...MockOpenPreviewResult, preview_slug: undefined },
		},
		{
			name: "an unknown kind",
			value: { ...MockOpenPreviewResult, kind: "tunnel" },
		},
		{
			name: "a preview slug outside the preview grammar",
			value: { ...MockOpenPreviewResult, preview_slug: "evil.example.com/" },
		},
		{
			name: "a port slug with a leading zero",
			value: { ...MockOpenPreviewResult, preview_slug: "preview--03000" },
		},
		{ name: "an empty title", value: { ...MockOpenPreviewResult, title: "" } },
		{
			name: "a title over 80 characters",
			value: { ...MockOpenPreviewResult, title: "a".repeat(81) },
		},
		{
			name: "a title that reverses its text",
			value: { ...MockOpenPreviewResult, title: "\u202Etis scoD" },
		},
		{
			name: "a title with an invisible character",
			value: { ...MockOpenPreviewResult, title: "Docs\u200Bsite" },
		},
		{
			name: "a title with a line break",
			value: { ...MockOpenPreviewResult, title: "Docs\u2028site" },
		},
		{
			name: "a URL path without a leading slash",
			value: { ...MockOpenPreviewResult, url_path: "docs" },
		},
		{
			name: "a URL path that names another host",
			value: { ...MockOpenPreviewResult, url_path: "//evil.example.com/" },
		},
		{
			name: "a URL path with a backslash",
			value: { ...MockOpenPreviewResult, url_path: "/\\evil.example.com" },
		},
		{
			name: "a URL path with a fragment",
			value: { ...MockOpenPreviewResult, url_path: "/docs#top" },
		},
		{
			name: "a URL path with a control character",
			value: { ...MockOpenPreviewResult, url_path: "/docs\n" },
		},
		{
			name: "a URL path over 2048 bytes",
			value: { ...MockOpenPreviewResult, url_path: `/${"é".repeat(1024)}` },
		},
	])("rejects $name", ({ value }) => {
		expect(parseOpenPreviewResult(value)).toBeUndefined();
	});

	it("counts a title's characters, not its UTF-16 code units", () => {
		const title = "🚀".repeat(80);

		expect(
			parseOpenPreviewResult({ ...MockOpenPreviewResult, title }),
		).toMatchObject({ title });
	});
});

describe("getPreviewPanelContextValue", () => {
	const getOpenAction = (enabled: boolean, result: Record<string, unknown>) => {
		const openFrameTab = vi.fn();
		const preview = parseOpenPreviewResult(result);
		if (!preview) {
			throw new Error("expected a valid open_preview result");
		}
		const open = getPreviewPanelContextValue({
			enabled,
			workspace: MockWorkspace,
			workspaceAgent: MockWorkspaceAgent,
			openFrameTab,
			focusPanel: vi.fn(),
		}).getOpenAction(preview);
		return { open, openFrameTab };
	};

	it("opens a preview tab for the chat's agent", () => {
		const { open, openFrameTab } = getOpenAction(true, MockOpenPreviewResult);

		open?.();

		expect(openFrameTab).toHaveBeenCalledWith(
			expect.objectContaining({
				kind: "preview",
				agentName: MockWorkspaceAgent.name,
				slug: MockOpenPreviewResult.preview_slug,
			}),
		);
	});

	it.each([
		{
			name: "while the open-preview experiment is off",
			enabled: false,
			result: MockOpenPreviewResult,
		},
		{
			name: "for another agent",
			enabled: true,
			result: { ...MockOpenPreviewResult, agent_name: "other-agent" },
		},
	])("offers no open action $name", ({ enabled, result }) => {
		expect(getOpenAction(enabled, result).open).toBeUndefined();
	});
});
