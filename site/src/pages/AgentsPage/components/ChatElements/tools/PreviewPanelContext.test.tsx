import { renderHook } from "@testing-library/react";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { createChatStore } from "../../ChatConversation/chatStore";
import { useOpenNewPreviews } from "./PreviewPanelContext";

const mockPreviewMessage: ChatMessage = {
	...MockChatMessage,
	id: 10,
	role: "tool",
	content: [
		{
			type: "tool-result",
			tool_call_id: "call-1",
			tool_name: "open_preview",
			result: {
				ok: true,
				workspace_id: "workspace-1",
				agent_name: "main",
				port: 3000,
			},
		},
	],
};

describe("useOpenNewPreviews", () => {
	it("opens previews from new messages only", async () => {
		const store = createChatStore();
		store.replaceMessages([mockPreviewMessage]);

		const onOpenPreview = vi.fn();
		renderHook(() =>
			useOpenNewPreviews({
				store,
				chatID: MockChatMessage.chat_id,
				initialMessages: [mockPreviewMessage],
				onOpenPreview,
			}),
		);

		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 9 });
			store.upsertDurableMessage({
				...mockPreviewMessage,
				id: 11,
				chat_id: "other-chat",
			});
		});

		// A send response can land before a lower-ID result from the stream.
		await act(async () => {
			store.upsertDurableMessage({ ...MockChatMessage, id: 13 });
		});
		expect(onOpenPreview).not.toHaveBeenCalled();

		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 12 });
		});

		// Reconnects can deliver a message again.
		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 12 });
		});

		expect(onOpenPreview).toHaveBeenCalledOnce();
		expect(onOpenPreview).toHaveBeenCalledWith({
			workspaceId: "workspace-1",
			preview: { agent_name: "main", port: 3000 },
		});
	});

	it("opens only the newest target of a batch", async () => {
		const store = createChatStore();
		const onOpenPreview = vi.fn();
		renderHook(() =>
			useOpenNewPreviews({
				store,
				chatID: MockChatMessage.chat_id,
				initialMessages: [],
				onOpenPreview,
			}),
		);

		const withPort = (id: number, port: number): ChatMessage => ({
			...mockPreviewMessage,
			id,
			content: [
				{
					type: "tool-result",
					tool_call_id: `call-${id}`,
					tool_name: "open_preview",
					result: {
						ok: true,
						workspace_id: "workspace-1",
						agent_name: "main",
						port,
					},
				},
			],
		});

		// Both land before a render, the newer one first.
		await act(async () => {
			store.upsertDurableMessage(withPort(21, 4000));
			store.upsertDurableMessage(withPort(20, 3000));
		});

		expect(onOpenPreview).toHaveBeenCalledOnce();
		expect(onOpenPreview).toHaveBeenCalledWith({
			workspaceId: "workspace-1",
			preview: { agent_name: "main", port: 4000 },
		});
	});
});
