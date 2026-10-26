import { renderHook } from "@testing-library/react";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatMessage } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { MockWorkspace } from "#/testHelpers/entities";
import { createChatStore } from "../../ChatConversation/chatStore";
import { useChatPreviewPanel, useNewPreviews } from "./PreviewPanelContext";

vi.mock("#/components/Toaster/toast", () => ({
	toast: { info: vi.fn(), dismiss: vi.fn() },
}));

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
				title: "Docs site",
			},
		},
	],
};

describe("useNewPreviews", () => {
	it("reports previews from new messages only", async () => {
		const store = createChatStore();
		store.replaceMessages([mockPreviewMessage]);

		const onNewPreview = vi.fn();
		renderHook(() =>
			useNewPreviews({
				store,
				initialMessages: [mockPreviewMessage],
				onNewPreview,
			}),
		);

		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 9 });
		});

		// A send response can land before a lower-ID result from the stream.
		await act(async () => {
			store.upsertDurableMessage({ ...MockChatMessage, id: 13 });
		});
		expect(onNewPreview).not.toHaveBeenCalled();

		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 12 });
		});

		// Reconnects can deliver a message again.
		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 12 });
		});

		expect(onNewPreview).toHaveBeenCalledOnce();
		expect(onNewPreview).toHaveBeenCalledWith(
			{
				workspaceId: "workspace-1",
				preview: { agent_name: "main", port: 3000 },
			},
			"Docs site",
		);
	});

	it("reports only the newest target", async () => {
		const store = createChatStore();
		const onNewPreview = vi.fn();
		renderHook(() =>
			useNewPreviews({
				store,
				initialMessages: [],
				onNewPreview,
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

		expect(onNewPreview).toHaveBeenCalledOnce();
		expect(onNewPreview).toHaveBeenCalledWith(
			{
				workspaceId: "workspace-1",
				preview: { agent_name: "main", port: 4000 },
			},
			"",
		);

		// A late older result must not replace the newer preview.
		await act(async () => {
			store.upsertDurableMessage(withPort(19, 5000));
		});
		expect(onNewPreview).toHaveBeenCalledOnce();
	});

	it("reports a preview that arrived before it could open", async () => {
		const store = createChatStore();
		const onNewPreview = vi.fn();
		const { rerender } = renderHook(
			({ canOpen }) =>
				useNewPreviews({
					store,
					initialMessages: [],
					onNewPreview: canOpen ? onNewPreview : undefined,
				}),
			{ initialProps: { canOpen: false } },
		);

		await act(async () => {
			store.upsertDurableMessage(mockPreviewMessage);
		});
		rerender({ canOpen: true });

		expect(onNewPreview).toHaveBeenCalledOnce();
	});
});

describe("useChatPreviewPanel", () => {
	it("offers new previews in a toast", async () => {
		const store = createChatStore();
		const showSingletonTab = vi.fn();
		const { rerender } = renderHook(
			({ workspaceId }) =>
				useChatPreviewPanel({
					chatId: "chat-1",
					store,
					initialMessages: [],
					workspace: { ...MockWorkspace, id: workspaceId },
					isPreviewTabSupported: true,
					isArchived: false,
					showSingletonTab,
					setPreviewPanelKey: vi.fn(),
				}),
			{ initialProps: { workspaceId: "workspace-1" } },
		);

		await act(async () => {
			store.upsertDurableMessage({
				...mockPreviewMessage,
				id: 9,
				content: [
					{
						type: "tool-result",
						tool_call_id: "call-other",
						tool_name: "open_preview",
						result: {
							ok: true,
							workspace_id: "workspace-2",
							agent_name: "main",
						},
					},
				],
			});
		});
		// The chat cannot show a preview from another workspace.
		expect(toast.info).not.toHaveBeenCalled();

		await act(async () => {
			store.upsertDurableMessage(mockPreviewMessage);
		});
		expect(showSingletonTab).not.toHaveBeenCalled();
		expect(toast.info).toHaveBeenCalledOnce();
		const [message, options] = vi.mocked(toast.info).mock.calls[0] as [
			string,
			{
				id: string;
				description: string;
				action: { label: string; onClick: () => void };
			},
		];
		expect(message).toBe("Opened a preview");
		expect(options.description).toBe("Docs site");
		expect(options.action.label).toBe("Open preview");
		act(() => options.action.onClick());
		expect(showSingletonTab).toHaveBeenCalledExactlyOnceWith("preview");

		rerender({ workspaceId: "workspace-2" });
		expect(toast.dismiss).toHaveBeenCalledWith(options.id);
	});
});
