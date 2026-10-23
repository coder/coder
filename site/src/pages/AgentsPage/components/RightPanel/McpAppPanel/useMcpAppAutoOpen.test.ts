import { renderHook } from "@testing-library/react";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChatMCPApp, MockChatMessage } from "#/testHelpers/chatEntities";
import { createChatStore } from "../../ChatConversation/chatStore";
import { useMcpAppAutoOpen } from "./useMcpAppAutoOpen";

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
			mcp_app: MockChatMCPApp,
		},
	],
});

describe("useMcpAppAutoOpen", () => {
	it("opens each new persisted result once and skips results from before mount", () => {
		const store = createChatStore();
		store.replaceMessages([mockToolResultMessage(5, "call-old")]);
		const onOpen = vi.fn();
		const newestMessageIdAtMount = 5;
		renderHook(() =>
			useMcpAppAutoOpen(store, true, newestMessageIdAtMount, onOpen),
		);

		act(() => {
			store.upsertDurableMessage(mockToolResultMessage(6, "call-new"));
		});
		act(() => {
			store.upsertDurableMessage({ ...MockChatMessage, id: 7 });
		});

		expect(onOpen).toHaveBeenCalledTimes(1);
		expect(onOpen).toHaveBeenCalledWith("call-new", MockChatMCPApp);
	});

	it("opens nothing while disabled", () => {
		const store = createChatStore();
		const onOpen = vi.fn();
		const newestMessageIdAtMount = 0;
		renderHook(() =>
			useMcpAppAutoOpen(store, false, newestMessageIdAtMount, onOpen),
		);

		act(() => {
			store.upsertDurableMessage(mockToolResultMessage(1, "call-1"));
		});

		expect(onOpen).not.toHaveBeenCalled();
	});
});
