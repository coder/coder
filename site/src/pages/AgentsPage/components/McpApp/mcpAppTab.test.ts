import { describe, expect, it } from "vitest";
import type { ChatMessage } from "#/api/typesGenerated";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { createChatStore } from "../ChatConversation/chatStore";
import {
	applyMcpAppTabUpserts,
	collectMcpAppToolCalls,
	type McpAppTab,
	type McpAppToolCallRef,
	mcpAppTabId,
	planMcpAppTabs,
} from "./mcpAppTab";

const SERVER = "mcp-1";
const URI = "ui://taskboard/board";
const TAB_ID = mcpAppTabId(SERVER, URI);

const ref = (toolCallId: string): McpAppToolCallRef => ({
	toolCallId,
	mcpServerConfigId: SERVER,
	resourceUri: URI,
	toolName: "add_task",
});

const appTab = (toolCallId: string): McpAppTab => ({
	id: TAB_ID,
	kind: "mcp_app",
	mcpServerConfigId: SERVER,
	resourceUri: URI,
	toolCallId,
	label: "Task board",
});

const terminalTab: UserRightPanelTab = {
	id: "terminal-1",
	kind: "terminal",
	reconnectionToken: "token",
};

const message = (
	id: number,
	toolCallId: string,
	uri?: string,
): ChatMessage => ({
	id,
	chat_id: "chat-1",
	created_at: "2026-01-01T00:00:00Z",
	role: "assistant",
	content: [
		{
			type: "tool-call",
			tool_call_id: toolCallId,
			tool_name: "add_task",
			args: {},
			mcp_server_config_id: SERVER,
			mcp_app_resource_uri: uri,
		},
	],
});

describe("collectMcpAppToolCalls", () => {
	it("lists persisted and live app tool calls in order and skips plain tools", () => {
		const store = createChatStore();
		store.replaceMessages([
			message(1, "call-1", URI),
			message(2, "call-plain"),
			message(3, "call-2", URI),
		]);
		store.applyMessagePart({
			type: "tool-call",
			tool_call_id: "call-3",
			tool_name: "add_task",
			args_delta: "{",
			mcp_server_config_id: SERVER,
			mcp_app_resource_uri: URI,
		});
		const state = store.getSnapshot();

		const refs = collectMcpAppToolCalls(
			state.messagesByID,
			state.orderedMessageIDs,
			state.streamState,
		);

		expect(refs.map((entry) => entry.toolCallId)).toEqual([
			"call-1",
			"call-2",
			"call-3",
		]);
	});

	it("returns the same array while the inputs are unchanged", () => {
		const store = createChatStore();
		store.replaceMessages([message(1, "call-1", URI)]);
		const state = store.getSnapshot();
		const first = collectMcpAppToolCalls(
			state.messagesByID,
			state.orderedMessageIDs,
			state.streamState,
		);
		const second = collectMcpAppToolCalls(
			state.messagesByID,
			state.orderedMessageIDs,
			state.streamState,
		);
		expect(second).toBe(first);
	});
});

describe("planMcpAppTabs", () => {
	const plan = (
		overrides: Partial<Parameters<typeof planMcpAppTabs>[0]> = {},
	) =>
		planMcpAppTabs({
			refs: [ref("call-1")],
			tabs: [terminalTab],
			dismissed: new Map(),
			initial: false,
			panelOpen: true,
			activeTabId: "terminal-1",
			labelFor: () => "Task board",
			...overrides,
		});

	it("creates a tab for a new app and selects it while the panel is open", () => {
		expect(plan()).toEqual({
			upserts: [appTab("call-1")],
			activateTabId: TAB_ID,
			badgeTabIds: [],
		});
	});

	it("badges instead of selecting when the panel is closed", () => {
		expect(plan({ panelOpen: false })).toEqual({
			upserts: [appTab("call-1")],
			badgeTabIds: [TAB_ID],
		});
	});

	it("rebinds an existing tab to the newest call and badges it", () => {
		const result = plan({
			refs: [ref("call-1"), ref("call-2")],
			tabs: [terminalTab, appTab("call-1")],
		});
		expect(result).toEqual({
			upserts: [appTab("call-2")],
			badgeTabIds: [TAB_ID],
		});
	});

	it("does not badge the tab that is already active", () => {
		const result = plan({
			refs: [ref("call-2")],
			tabs: [appTab("call-1")],
			activeTabId: TAB_ID,
		});
		expect(result).toEqual({ upserts: [appTab("call-2")], badgeTabIds: [] });
	});

	it("creates tabs silently for history loaded with the page", () => {
		expect(plan({ initial: true })).toEqual({
			upserts: [appTab("call-1")],
			badgeTabIds: [],
		});
	});

	it("does nothing when the tab already shows the newest call", () => {
		expect(plan({ tabs: [appTab("call-1")] })).toEqual({
			upserts: [],
			badgeTabIds: [],
		});
	});

	it("leaves a closed tab closed until a different call arrives", () => {
		const dismissed = new Map([[TAB_ID, "call-1"]]);
		expect(plan({ dismissed })).toEqual({ upserts: [], badgeTabIds: [] });
		expect(plan({ dismissed, refs: [ref("call-1"), ref("call-2")] })).toEqual({
			upserts: [appTab("call-2")],
			activateTabId: TAB_ID,
			badgeTabIds: [],
		});
	});
});

describe("applyMcpAppTabUpserts", () => {
	it("replaces an existing app tab in place and appends new ones", () => {
		const other: McpAppTab = {
			...appTab("call-9"),
			id: mcpAppTabId("mcp-2", URI),
			mcpServerConfigId: "mcp-2",
		};
		expect(
			applyMcpAppTabUpserts(
				[appTab("call-1"), terminalTab],
				[appTab("call-2"), other],
			),
		).toEqual([appTab("call-2"), terminalTab, other]);
	});
});
