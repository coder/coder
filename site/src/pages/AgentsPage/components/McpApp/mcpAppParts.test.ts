import { describe, expect, it } from "vitest";
import { MaxChatMCPAppContextBytes } from "#/api/typesGenerated";
import { createChatStore } from "../ChatConversation/chatStore";
import {
	buildMcpAppContextInputParts,
	flattenModelContext,
	takeMcpAppContextInputParts,
} from "./mcpAppParts";

describe("flattenModelContext", () => {
	it("joins text blocks and appends structured content as JSON", () => {
		expect(
			flattenModelContext({
				content: [
					{ type: "text", text: "line one" },
					{ type: "image", text: undefined },
					{ type: "text", text: "line two" },
				],
				structuredContent: { count: 2 },
			}),
		).toBe('line one\nline two\n{"count":2}');
	});

	it("returns an empty string for an empty update", () => {
		expect(flattenModelContext({})).toBe("");
	});

	it("truncates to the server cap without splitting a code point", () => {
		const text = "é".repeat(MaxChatMCPAppContextBytes);
		const flattened = flattenModelContext({
			content: [{ type: "text", text }],
		});
		const bytes = new TextEncoder().encode(flattened).length;
		expect(bytes).toBeLessThanOrEqual(MaxChatMCPAppContextBytes);
		expect(flattened).not.toContain("\uFFFD");
		expect(flattened.endsWith("é")).toBe(true);
	});
});

describe("takeMcpAppContextInputParts", () => {
	const seed = () => {
		const store = createChatStore();
		store.setMcpAppContext("app-1", {
			mcpServerConfigId: "mcp-1",
			resourceUri: "ui://taskboard/board",
			text: "Board has 1 task",
		});
		store.setMcpAppContext("app-2", {
			mcpServerConfigId: "mcp-2",
			resourceUri: "ui://other/view",
			text: "",
		});
		return store;
	};

	it("appends one part per app with text for a new send and clears the store", () => {
		const store = seed();

		const { parts } = takeMcpAppContextInputParts(store);

		expect(parts).toEqual([
			{
				type: "mcp-app-context",
				mcp_server_config_id: "mcp-1",
				mcp_app_resource_uri: "ui://taskboard/board",
				text: "Board has 1 task",
			},
		]);
		expect(store.getSnapshot().mcpAppContexts.size).toBe(0);
	});

	it("restore puts the taken contexts back after a failed send", () => {
		const store = seed();
		const { restore } = takeMcpAppContextInputParts(store);

		restore();

		expect(store.getSnapshot().mcpAppContexts.get("app-1")?.text).toBe(
			"Board has 1 task",
		);
		expect(store.getSnapshot().mcpAppContexts.size).toBe(2);
	});
});

describe("buildMcpAppContextInputParts", () => {
	it("skips entries without text", () => {
		expect(
			buildMcpAppContextInputParts([
				["a", { mcpServerConfigId: "m", resourceUri: "ui://x", text: "" }],
			]),
		).toEqual([]);
	});
});
