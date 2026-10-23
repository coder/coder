import { renderHook } from "@testing-library/react";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatMCPApp, ChatMessage } from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { createChatStore } from "../../ChatConversation/chatStore";
import { useMcpAppAutoOpen } from "./useMcpAppAutoOpen";

const mockMcpApp: ChatMCPApp = {
	mcp_server_config_id: "server-1",
	tool_name: "show_dice_board",
	title: "Dice board",
	resource_uri: "ui://dice/board.html",
	tool_input: {},
};

const mockToolResultMessage = (
	id: number,
	toolCallId: string,
): ChatMessage => ({
	...MockChatMessage,
	id,
	role: "tool",
	content: [
		{
			type: "tool-result",
			tool_call_id: toolCallId,
			tool_name: "dice__show_dice_board",
			mcp_app: mockMcpApp,
		},
	],
});

describe("useMcpAppAutoOpen", () => {
	it("opens each new persisted result once and skips results from before mount", () => {
		const store = createChatStore();
		store.replaceMessages([mockToolResultMessage(5, "call-old")]);
		const onOpen = vi.fn();
		renderHook(() => useMcpAppAutoOpen(store, true, 5, onOpen));

		act(() => {
			store.upsertDurableMessage(mockToolResultMessage(6, "call-new"));
		});
		act(() => {
			store.upsertDurableMessage({ ...MockChatMessage, id: 7 });
		});

		expect(onOpen).toHaveBeenCalledTimes(1);
		expect(onOpen).toHaveBeenCalledWith("call-new", mockMcpApp);
	});

	it("opens nothing while disabled", () => {
		const store = createChatStore();
		const onOpen = vi.fn();
		renderHook(() => useMcpAppAutoOpen(store, false, 0, onOpen));

		act(() => {
			store.upsertDurableMessage(mockToolResultMessage(1, "call-1"));
		});

		expect(onOpen).not.toHaveBeenCalled();
	});
});
